// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"

	// Import the sqlite3 package
	_ "github.com/mattn/go-sqlite3"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/syncapi/storage/shared"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type stateDelta struct {
	roomID      string
	stateEvents []gomatrixserverlib.HeaderedEvent
	membership  string
	// The PDU stream position of the latest membership event for this user, if applicable.
	// Can be 0 if there is no membership event in this delta.
	membershipPos types.StreamPosition
}

// SyncServerDatasource represents a sync server datasource which manages
// both the database for PDUs and caches for EDUs.
type SyncServerDatasource struct {
	shared.Database
	db *sql.DB
	common.PartitionOffsetStatements
	streamID streamIDStatements
}

// NewSyncServerDatasource creates a new sync server database
// nolint: gocyclo
func NewSyncServerDatasource(dataSourceName string) (*SyncServerDatasource, error) {
	var d SyncServerDatasource
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return nil, err
	}
	var cs string
	if uri.Opaque != "" { // file:filename.db
		cs = uri.Opaque
	} else if uri.Path != "" { // file:///path/to/filename.db
		cs = uri.Path
	} else {
		return nil, errors.New("no filename or path in connect string")
	}
	if d.db, err = sqlutil.Open(common.SQLiteDriverName(), cs, nil); err != nil {
		return nil, err
	}
	if err = d.prepare(); err != nil {
		return nil, err
	}
	return &d, nil
}

func (d *SyncServerDatasource) prepare() (err error) {
	if err = d.PartitionOffsetStatements.Prepare(d.db, "syncapi"); err != nil {
		return err
	}
	if err = d.streamID.prepare(d.db); err != nil {
		return err
	}
	accountData, err := NewSqliteAccountDataTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	events, err := NewSqliteEventsTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	roomState, err := NewSqliteCurrentRoomStateTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	invites, err := NewSqliteInvitesTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	topology, err := NewSqliteTopologyTable(d.db)
	if err != nil {
		return err
	}
	bwExtrem, err := NewSqliteBackwardsExtremitiesTable(d.db)
	if err != nil {
		return err
	}
	d.Database = shared.Database{
		DB:                  d.db,
		Invites:             invites,
		AccountData:         accountData,
		OutputEvents:        events,
		BackwardExtremities: bwExtrem,
		CurrentRoomState:    roomState,
		Topology:            topology,
		EDUCache:            cache.New(),
	}
	return nil
}

// handleBackwardExtremities adds this event as a backwards extremity if and only if we do not have all of
// the events listed in the event's 'prev_events'. This function also updates the backwards extremities table
// to account for the fact that the given event is no longer a backwards extremity, but may be marked as such.
func (d *SyncServerDatasource) handleBackwardExtremities(ctx context.Context, txn *sql.Tx, ev *gomatrixserverlib.HeaderedEvent) error {
	if err := d.Database.BackwardExtremities.DeleteBackwardExtremity(ctx, txn, ev.RoomID(), ev.EventID()); err != nil {
		return err
	}

	// Check if we have all of the event's previous events. If an event is
	// missing, add it to the room's backward extremities.
	prevEvents, err := d.Database.OutputEvents.SelectEvents(ctx, txn, ev.PrevEventIDs())
	if err != nil {
		return err
	}
	var found bool
	for _, eID := range ev.PrevEventIDs() {
		found = false
		for _, prevEv := range prevEvents {
			if eID == prevEv.EventID() {
				found = true
			}
		}

		// If the event is missing, consider it a backward extremity.
		if !found {
			if err = d.Database.BackwardExtremities.InsertsBackwardExtremity(ctx, txn, ev.RoomID(), ev.EventID(), eID); err != nil {
				return err
			}
		}
	}

	return nil
}

// WriteEvent into the database. It is not safe to call this function from multiple goroutines, as it would create races
// when generating the sync stream position for this event. Returns the sync stream position for the inserted event.
// Returns an error if there was a problem inserting this event.
func (d *SyncServerDatasource) WriteEvent(
	ctx context.Context,
	ev *gomatrixserverlib.HeaderedEvent,
	addStateEvents []gomatrixserverlib.HeaderedEvent,
	addStateEventIDs, removeStateEventIDs []string,
	transactionID *api.TransactionID, excludeFromSync bool,
) (pduPosition types.StreamPosition, returnErr error) {
	returnErr = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		var err error
		pos, err := d.Database.OutputEvents.InsertEvent(
			ctx, txn, ev, addStateEventIDs, removeStateEventIDs, transactionID, excludeFromSync,
		)
		if err != nil {
			return err
		}
		pduPosition = pos

		if err = d.Database.Topology.InsertEventInTopology(ctx, txn, ev, pos); err != nil {
			return err
		}

		if err = d.handleBackwardExtremities(ctx, txn, ev); err != nil {
			return err
		}

		if len(addStateEvents) == 0 && len(removeStateEventIDs) == 0 {
			// Nothing to do, the event may have just been a message event.
			return nil
		}

		return d.updateRoomState(ctx, txn, removeStateEventIDs, addStateEvents, pduPosition)
	})

	return pduPosition, returnErr
}

func (d *SyncServerDatasource) updateRoomState(
	ctx context.Context, txn *sql.Tx,
	removedEventIDs []string,
	addedEvents []gomatrixserverlib.HeaderedEvent,
	pduPosition types.StreamPosition,
) error {
	// remove first, then add, as we do not ever delete state, but do replace state which is a remove followed by an add.
	for _, eventID := range removedEventIDs {
		if err := d.Database.CurrentRoomState.DeleteRoomStateByEventID(ctx, txn, eventID); err != nil {
			return err
		}
	}

	for _, event := range addedEvents {
		if event.StateKey() == nil {
			// ignore non state events
			continue
		}
		var membership *string
		if event.Type() == "m.room.member" {
			value, err := event.Membership()
			if err != nil {
				return err
			}
			membership = &value
		}
		if err := d.Database.CurrentRoomState.UpsertRoomState(ctx, txn, event, membership, pduPosition); err != nil {
			return err
		}
	}

	return nil
}

// SyncPosition returns the latest positions for syncing.
func (d *SyncServerDatasource) SyncPosition(ctx context.Context) (tok types.StreamingToken, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		pos, err := d.syncPositionTx(ctx, txn)
		if err != nil {
			return err
		}
		tok = *pos
		return nil
	})
	return
}

// GetEventsInTopologicalRange retrieves all of the events on a given ordering using the
// given extremities and limit.
func (d *SyncServerDatasource) GetEventsInTopologicalRange(
	ctx context.Context,
	from, to *types.TopologyToken,
	roomID string, limit int,
	backwardOrdering bool,
) (events []types.StreamEvent, err error) {
	// TODO: ARGH CONFUSING
	// Determine the backward and forward limit, i.e. the upper and lower
	// limits to the selection in the room's topology, from the direction.
	var backwardLimit, forwardLimit, forwardMicroLimit types.StreamPosition
	if backwardOrdering {
		// Backward ordering is antichronological (latest event to oldest
		// one).
		backwardLimit = to.Depth()
		forwardLimit = from.Depth()
		forwardMicroLimit = from.PDUPosition()
	} else {
		// Forward ordering is chronological (oldest event to latest one).
		backwardLimit = from.Depth()
		forwardLimit = to.Depth()
	}

	// Select the event IDs from the defined range.
	var eIDs []string
	eIDs, err = d.Database.Topology.SelectEventIDsInRange(
		ctx, nil, roomID, backwardLimit, forwardLimit, forwardMicroLimit, limit, !backwardOrdering,
	)
	if err != nil {
		return
	}

	// Retrieve the events' contents using their IDs.
	events, err = d.Database.OutputEvents.SelectEvents(ctx, nil, eIDs)
	return
}

// MaxTopologicalPosition returns the highest topological position for a given
// room.
func (d *SyncServerDatasource) MaxTopologicalPosition(
	ctx context.Context, roomID string,
) (types.StreamPosition, types.StreamPosition, error) {
	return d.Database.Topology.SelectMaxPositionInTopology(ctx, nil, roomID)
}

// EventsAtTopologicalPosition returns all of the events matching a given
// position in the topology of a given room.
func (d *SyncServerDatasource) EventsAtTopologicalPosition(
	ctx context.Context, roomID string, pos types.StreamPosition,
) ([]types.StreamEvent, error) {
	eIDs, err := d.Database.Topology.SelectEventIDsFromPosition(ctx, nil, roomID, pos)
	if err != nil {
		return nil, err
	}

	return d.Database.OutputEvents.SelectEvents(ctx, nil, eIDs)
}

func (d *SyncServerDatasource) EventPositionInTopology(
	ctx context.Context, eventID string,
) (depth types.StreamPosition, stream types.StreamPosition, err error) {
	return d.Database.Topology.SelectPositionInTopology(ctx, nil, eventID)
}

// SyncStreamPosition returns the latest position in the sync stream. Returns 0 if there are no events yet.
func (d *SyncServerDatasource) SyncStreamPosition(ctx context.Context) (pos types.StreamPosition, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		pos, err = d.syncStreamPositionTx(ctx, txn)
		return err
	})
	return
}

func (d *SyncServerDatasource) syncStreamPositionTx(
	ctx context.Context, txn *sql.Tx,
) (types.StreamPosition, error) {
	maxID, err := d.Database.OutputEvents.SelectMaxEventID(ctx, txn)
	if err != nil {
		return 0, err
	}
	maxAccountDataID, err := d.Database.AccountData.SelectMaxAccountDataID(ctx, txn)
	if err != nil {
		return 0, err
	}
	if maxAccountDataID > maxID {
		maxID = maxAccountDataID
	}
	maxInviteID, err := d.Database.Invites.SelectMaxInviteID(ctx, txn)
	if err != nil {
		return 0, err
	}
	if maxInviteID > maxID {
		maxID = maxInviteID
	}
	return types.StreamPosition(maxID), nil
}

func (d *SyncServerDatasource) syncPositionTx(
	ctx context.Context, txn *sql.Tx,
) (*types.StreamingToken, error) {

	maxEventID, err := d.Database.OutputEvents.SelectMaxEventID(ctx, txn)
	if err != nil {
		return nil, err
	}
	maxAccountDataID, err := d.Database.AccountData.SelectMaxAccountDataID(ctx, txn)
	if err != nil {
		return nil, err
	}
	if maxAccountDataID > maxEventID {
		maxEventID = maxAccountDataID
	}
	maxInviteID, err := d.Database.Invites.SelectMaxInviteID(ctx, txn)
	if err != nil {
		return nil, err
	}
	if maxInviteID > maxEventID {
		maxEventID = maxInviteID
	}
	sp := types.NewStreamToken(
		types.StreamPosition(maxEventID),
		types.StreamPosition(d.Database.EDUCache.GetLatestSyncPosition()),
	)
	return &sp, nil
}

// addPDUDeltaToResponse adds all PDU deltas to a sync response.
// IDs of all rooms the user joined are returned so EDU deltas can be added for them.
func (d *SyncServerDatasource) addPDUDeltaToResponse(
	ctx context.Context,
	device authtypes.Device,
	fromPos, toPos types.StreamPosition,
	numRecentEventsPerRoom int,
	wantFullState bool,
	res *types.Response,
) (joinedRoomIDs []string, err error) {
	txn, err := d.db.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return nil, err
	}
	var succeeded bool
	defer func() {
		txerr := common.EndTransaction(txn, &succeeded)
		if err == nil && txerr != nil {
			err = txerr
		}
	}()

	stateFilterPart := gomatrixserverlib.DefaultStateFilter() // TODO: use filter provided in request

	// Work out which rooms to return in the response. This is done by getting not only the currently
	// joined rooms, but also which rooms have membership transitions for this user between the 2 PDU stream positions.
	// This works out what the 'state' key should be for each room as well as which membership block
	// to put the room into.
	var deltas []stateDelta
	if !wantFullState {
		deltas, joinedRoomIDs, err = d.getStateDeltas(
			ctx, &device, txn, fromPos, toPos, device.UserID, &stateFilterPart,
		)
	} else {
		deltas, joinedRoomIDs, err = d.getStateDeltasForFullStateSync(
			ctx, &device, txn, fromPos, toPos, device.UserID, &stateFilterPart,
		)
	}
	if err != nil {
		return nil, err
	}

	for _, delta := range deltas {
		err = d.addRoomDeltaToResponse(ctx, &device, txn, fromPos, toPos, delta, numRecentEventsPerRoom, res)
		if err != nil {
			return nil, err
		}
	}

	// TODO: This should be done in getStateDeltas
	if err = d.addInvitesToResponse(ctx, txn, device.UserID, fromPos, toPos, res); err != nil {
		return nil, err
	}

	succeeded = true
	return joinedRoomIDs, nil
}

// addTypingDeltaToResponse adds all typing notifications to a sync response
// since the specified position.
func (d *SyncServerDatasource) addTypingDeltaToResponse(
	since types.StreamingToken,
	joinedRoomIDs []string,
	res *types.Response,
) error {
	var jr types.JoinResponse
	var ok bool
	var err error
	for _, roomID := range joinedRoomIDs {
		if typingUsers, updated := d.Database.EDUCache.GetTypingUsersIfUpdatedAfter(
			roomID, int64(since.EDUPosition()),
		); updated {
			ev := gomatrixserverlib.ClientEvent{
				Type: gomatrixserverlib.MTyping,
			}
			ev.Content, err = json.Marshal(map[string]interface{}{
				"user_ids": typingUsers,
			})
			if err != nil {
				return err
			}

			if jr, ok = res.Rooms.Join[roomID]; !ok {
				jr = *types.NewJoinResponse()
			}
			jr.Ephemeral.Events = append(jr.Ephemeral.Events, ev)
			res.Rooms.Join[roomID] = jr
		}
	}
	return nil
}

// addEDUDeltaToResponse adds updates for EDUs of each type since fromPos if
// the positions of that type are not equal in fromPos and toPos.
func (d *SyncServerDatasource) addEDUDeltaToResponse(
	fromPos, toPos types.StreamingToken,
	joinedRoomIDs []string,
	res *types.Response,
) (err error) {

	if fromPos.EDUPosition() != toPos.EDUPosition() {
		err = d.addTypingDeltaToResponse(
			fromPos, joinedRoomIDs, res,
		)
	}

	return
}

// IncrementalSync returns all the data needed in order to create an incremental
// sync response for the given user. Events returned will include any client
// transaction IDs associated with the given device. These transaction IDs come
// from when the device sent the event via an API that included a transaction
// ID.
func (d *SyncServerDatasource) IncrementalSync(
	ctx context.Context,
	device authtypes.Device,
	fromPos, toPos types.StreamingToken,
	numRecentEventsPerRoom int,
	wantFullState bool,
) (*types.Response, error) {
	nextBatchPos := fromPos.WithUpdates(toPos)
	res := types.NewResponse(nextBatchPos)

	var joinedRoomIDs []string
	var err error
	fmt.Println("from", fromPos.PDUPosition(), "to", toPos.PDUPosition())
	if fromPos.PDUPosition() != toPos.PDUPosition() || wantFullState {
		joinedRoomIDs, err = d.addPDUDeltaToResponse(
			ctx, device, fromPos.PDUPosition(), toPos.PDUPosition(), numRecentEventsPerRoom, wantFullState, res,
		)
	} else {
		joinedRoomIDs, err = d.Database.CurrentRoomState.SelectRoomIDsWithMembership(
			ctx, nil, device.UserID, gomatrixserverlib.Join,
		)
	}
	if err != nil {
		return nil, err
	}

	err = d.addEDUDeltaToResponse(
		fromPos, toPos, joinedRoomIDs, res,
	)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// getResponseWithPDUsForCompleteSync creates a response and adds all PDUs needed
// to it. It returns toPos and joinedRoomIDs for use of adding EDUs.
func (d *SyncServerDatasource) getResponseWithPDUsForCompleteSync(
	ctx context.Context,
	userID string,
	numRecentEventsPerRoom int,
) (
	res *types.Response,
	toPos *types.StreamingToken,
	joinedRoomIDs []string,
	err error,
) {
	// This needs to be all done in a transaction as we need to do multiple SELECTs, and we need to have
	// a consistent view of the database throughout. This includes extracting the sync position.
	// This does have the unfortunate side-effect that all the matrixy logic resides in this function,
	// but it's better to not hide the fact that this is being done in a transaction.
	txn, err := d.db.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return
	}
	var succeeded bool
	defer func() {
		txerr := common.EndTransaction(txn, &succeeded)
		if err == nil && txerr != nil {
			err = txerr
		}
	}()

	// Get the current sync position which we will base the sync response on.
	toPos, err = d.syncPositionTx(ctx, txn)
	if err != nil {
		return
	}

	res = types.NewResponse(*toPos)

	// Extract room state and recent events for all rooms the user is joined to.
	joinedRoomIDs, err = d.Database.CurrentRoomState.SelectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
	if err != nil {
		return
	}

	stateFilterPart := gomatrixserverlib.DefaultStateFilter() // TODO: use filter provided in request

	// Build up a /sync response. Add joined rooms.
	for _, roomID := range joinedRoomIDs {
		var stateEvents []gomatrixserverlib.HeaderedEvent
		stateEvents, err = d.Database.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, &stateFilterPart)
		if err != nil {
			return
		}
		//fmt.Println("State events:", stateEvents)
		// TODO: When filters are added, we may need to call this multiple times to get enough events.
		//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
		var recentStreamEvents []types.StreamEvent
		recentStreamEvents, err = d.Database.OutputEvents.SelectRecentEvents(
			ctx, txn, roomID, types.StreamPosition(0), toPos.PDUPosition(),
			numRecentEventsPerRoom, true, true,
		)
		if err != nil {
			return
		}
		//fmt.Println("Recent stream events:", recentStreamEvents)

		// Retrieve the backward topology position, i.e. the position of the
		// oldest event in the room's topology.
		var prevBatchStr string
		if len(recentStreamEvents) > 0 {
			var backwardTopologyPos, backwardStreamPos types.StreamPosition
			backwardTopologyPos, backwardStreamPos, err = d.Database.Topology.SelectPositionInTopology(ctx, txn, recentStreamEvents[0].EventID())
			if err != nil {
				return
			}
			prevBatch := types.NewTopologyToken(backwardTopologyPos, backwardStreamPos)
			prevBatch.Decrement()
			prevBatchStr = prevBatch.String()
		}

		// We don't include a device here as we don't need to send down
		// transaction IDs for complete syncs
		recentEvents := d.StreamEventsToEvents(nil, recentStreamEvents)
		stateEvents = removeDuplicates(stateEvents, recentEvents)
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = prevBatchStr
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = true
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[roomID] = *jr
	}

	if err = d.addInvitesToResponse(ctx, txn, userID, 0, toPos.PDUPosition(), res); err != nil {
		return
	}

	succeeded = true
	return res, toPos, joinedRoomIDs, err
}

// CompleteSync returns a complete /sync API response for the given user.
func (d *SyncServerDatasource) CompleteSync(
	ctx context.Context, userID string, numRecentEventsPerRoom int,
) (*types.Response, error) {
	res, toPos, joinedRoomIDs, err := d.getResponseWithPDUsForCompleteSync(
		ctx, userID, numRecentEventsPerRoom,
	)
	if err != nil {
		return nil, err
	}

	// Use a zero value SyncPosition for fromPos so all EDU states are added.
	err = d.addEDUDeltaToResponse(
		types.NewStreamToken(0, 0), *toPos, joinedRoomIDs, res,
	)
	if err != nil {
		return nil, err
	}

	return res, nil
}

var txReadOnlySnapshot = sql.TxOptions{
	// Set the isolation level so that we see a snapshot of the database.
	// In PostgreSQL repeatable read transactions will see a snapshot taken
	// at the first query, and since the transaction is read-only it can't
	// run into any serialisation errors.
	// https://www.postgresql.org/docs/9.5/static/transaction-iso.html#XACT-REPEATABLE-READ
	Isolation: sql.LevelRepeatableRead,
	ReadOnly:  true,
}

func (d *SyncServerDatasource) addInvitesToResponse(
	ctx context.Context, txn *sql.Tx,
	userID string,
	fromPos, toPos types.StreamPosition,
	res *types.Response,
) error {
	invites, err := d.Database.Invites.SelectInviteEventsInRange(
		ctx, txn, userID, fromPos, toPos,
	)
	if err != nil {
		return err
	}
	for roomID, inviteEvent := range invites {
		ir := types.NewInviteResponse(inviteEvent)
		res.Rooms.Invite[roomID] = *ir
	}
	return nil
}

// Retrieve the backward topology position, i.e. the position of the
// oldest event in the room's topology.
func (d *SyncServerDatasource) getBackwardTopologyPos(
	ctx context.Context, txn *sql.Tx,
	events []types.StreamEvent,
) (pos, spos types.StreamPosition) {
	if len(events) > 0 {
		pos, spos, _ = d.Database.Topology.SelectPositionInTopology(ctx, txn, events[0].EventID())
	}
	// go to the previous position so we don't pull out the same event twice
	// FIXME: This could be done more nicely by being explicit with inclusive/exclusive rules
	if pos-1 <= 0 {
		pos = types.StreamPosition(1)
	} else {
		pos = pos - 1
		spos += 1000 // this has to be bigger than the number of events we backfill per request
	}
	return
}

// addRoomDeltaToResponse adds a room state delta to a sync response
func (d *SyncServerDatasource) addRoomDeltaToResponse(
	ctx context.Context,
	device *authtypes.Device,
	txn *sql.Tx,
	fromPos, toPos types.StreamPosition,
	delta stateDelta,
	numRecentEventsPerRoom int,
	res *types.Response,
) error {
	endPos := toPos
	if delta.membershipPos > 0 && delta.membership == gomatrixserverlib.Leave {
		// make sure we don't leak recent events after the leave event.
		// TODO: History visibility makes this somewhat complex to handle correctly. For example:
		// TODO: This doesn't work for join -> leave in a single /sync request (see events prior to join).
		// TODO: This will fail on join -> leave -> sensitive msg -> join -> leave
		//       in a single /sync request
		// This is all "okay" assuming history_visibility == "shared" which it is by default.
		endPos = delta.membershipPos
	}
	recentStreamEvents, err := d.Database.OutputEvents.SelectRecentEvents(
		ctx, txn, delta.roomID, types.StreamPosition(fromPos), types.StreamPosition(endPos),
		numRecentEventsPerRoom, true, true,
	)
	if err != nil {
		return err
	}
	recentEvents := d.StreamEventsToEvents(device, recentStreamEvents)
	delta.stateEvents = removeDuplicates(delta.stateEvents, recentEvents)
	backwardTopologyPos, backwardStreamPos := d.getBackwardTopologyPos(ctx, txn, recentStreamEvents)
	prevBatch := types.NewTopologyToken(
		backwardTopologyPos, backwardStreamPos,
	)

	switch delta.membership {
	case gomatrixserverlib.Join:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = prevBatch.String()
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[delta.roomID] = *jr
	case gomatrixserverlib.Leave:
		fallthrough // transitions to leave are the same as ban
	case gomatrixserverlib.Ban:
		// TODO: recentEvents may contain events that this user is not allowed to see because they are
		//       no longer in the room.
		lr := types.NewLeaveResponse()
		lr.Timeline.PrevBatch = prevBatch.String()
		lr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		lr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		lr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Leave[delta.roomID] = *lr
	}

	return nil
}

// fetchStateEvents converts the set of event IDs into a set of events. It will fetch any which are missing from the database.
// Returns a map of room ID to list of events.
func (d *SyncServerDatasource) fetchStateEvents(
	ctx context.Context, txn *sql.Tx,
	roomIDToEventIDSet map[string]map[string]bool,
	eventIDToEvent map[string]types.StreamEvent,
) (map[string][]types.StreamEvent, error) {
	stateBetween := make(map[string][]types.StreamEvent)
	missingEvents := make(map[string][]string)
	for roomID, ids := range roomIDToEventIDSet {
		events := stateBetween[roomID]
		for id, need := range ids {
			if !need {
				continue // deleted state
			}
			e, ok := eventIDToEvent[id]
			if ok {
				events = append(events, e)
			} else {
				m := missingEvents[roomID]
				m = append(m, id)
				missingEvents[roomID] = m
			}
		}
		stateBetween[roomID] = events
	}

	if len(missingEvents) > 0 {
		// This happens when add_state_ids has an event ID which is not in the provided range.
		// We need to explicitly fetch them.
		allMissingEventIDs := []string{}
		for _, missingEvIDs := range missingEvents {
			allMissingEventIDs = append(allMissingEventIDs, missingEvIDs...)
		}
		evs, err := d.fetchMissingStateEvents(ctx, txn, allMissingEventIDs)
		if err != nil {
			return nil, err
		}
		// we know we got them all otherwise an error would've been returned, so just loop the events
		for _, ev := range evs {
			roomID := ev.RoomID()
			stateBetween[roomID] = append(stateBetween[roomID], ev)
		}
	}
	return stateBetween, nil
}

func (d *SyncServerDatasource) fetchMissingStateEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	// Fetch from the events table first so we pick up the stream ID for the
	// event.
	events, err := d.Database.OutputEvents.SelectEvents(ctx, txn, eventIDs)
	if err != nil {
		return nil, err
	}

	have := map[string]bool{}
	for _, event := range events {
		have[event.EventID()] = true
	}
	var missing []string
	for _, eventID := range eventIDs {
		if !have[eventID] {
			missing = append(missing, eventID)
		}
	}
	if len(missing) == 0 {
		return events, nil
	}

	// If they are missing from the events table then they should be state
	// events that we received from outside the main event stream.
	// These should be in the room state table.
	stateEvents, err := d.Database.CurrentRoomState.SelectEventsWithEventIDs(ctx, txn, missing)

	if err != nil {
		return nil, err
	}
	if len(stateEvents) != len(missing) {
		return nil, fmt.Errorf("failed to map all event IDs to events: (got %d, wanted %d)", len(stateEvents), len(missing))
	}
	events = append(events, stateEvents...)
	return events, nil
}

// getStateDeltas returns the state deltas between fromPos and toPos,
// exclusive of oldPos, inclusive of newPos, for the rooms in which
// the user has new membership events.
// A list of joined room IDs is also returned in case the caller needs it.
func (d *SyncServerDatasource) getStateDeltas(
	ctx context.Context, device *authtypes.Device, txn *sql.Tx,
	fromPos, toPos types.StreamPosition, userID string,
	stateFilterPart *gomatrixserverlib.StateFilter,
) ([]stateDelta, []string, error) {
	// Implement membership change algorithm: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L821
	// - Get membership list changes for this user in this sync response
	// - For each room which has membership list changes:
	//     * Check if the room is 'newly joined' (insufficient to just check for a join event because we allow dupe joins TODO).
	//       If it is, then we need to send the full room state down (and 'limited' is always true).
	//     * Check if user is still CURRENTLY invited to the room. If so, add room to 'invited' block.
	//     * Check if the user is CURRENTLY (TODO) left/banned. If so, add room to 'archived' block.
	// - Get all CURRENTLY joined rooms, and add them to 'joined' block.
	var deltas []stateDelta

	// get all the state events ever between these two positions
	stateNeeded, eventMap, err := d.Database.OutputEvents.SelectStateInRange(ctx, txn, fromPos, toPos, stateFilterPart)
	if err != nil {
		return nil, nil, err
	}
	state, err := d.fetchStateEvents(ctx, txn, stateNeeded, eventMap)
	if err != nil {
		return nil, nil, err
	}

	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			// TODO: Currently this will incorrectly add rooms which were ALREADY joined but they sent another no-op join event.
			//       We should be checking if the user was already joined at fromPos and not proceed if so. As a result of this,
			//       dupe join events will result in the entire room state coming down to the client again. This is added in
			//       the 'state' part of the response though, so is transparent modulo bandwidth concerns as it is not added to
			//       the timeline.
			if membership := getMembershipFromEvent(&ev.HeaderedEvent, userID); membership != "" {
				if membership == gomatrixserverlib.Join {
					// send full room state down instead of a delta
					var s []types.StreamEvent
					s, err = d.currentStateStreamEventsForRoom(ctx, txn, roomID, stateFilterPart)
					if err != nil {
						return nil, nil, err
					}
					state[roomID] = s
					continue // we'll add this room in when we do joined rooms
				}

				deltas = append(deltas, stateDelta{
					membership:    membership,
					membershipPos: ev.StreamPosition,
					stateEvents:   d.StreamEventsToEvents(device, stateStreamEvents),
					roomID:        roomID,
				})
				break
			}
		}
	}

	// Add in currently joined rooms
	joinedRoomIDs, err := d.Database.CurrentRoomState.SelectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
	if err != nil {
		return nil, nil, err
	}
	for _, joinedRoomID := range joinedRoomIDs {
		deltas = append(deltas, stateDelta{
			membership:  gomatrixserverlib.Join,
			stateEvents: d.StreamEventsToEvents(device, state[joinedRoomID]),
			roomID:      joinedRoomID,
		})
	}

	return deltas, joinedRoomIDs, nil
}

// getStateDeltasForFullStateSync is a variant of getStateDeltas used for /sync
// requests with full_state=true.
// Fetches full state for all joined rooms and uses selectStateInRange to get
// updates for other rooms.
func (d *SyncServerDatasource) getStateDeltasForFullStateSync(
	ctx context.Context, device *authtypes.Device, txn *sql.Tx,
	fromPos, toPos types.StreamPosition, userID string,
	stateFilterPart *gomatrixserverlib.StateFilter,
) ([]stateDelta, []string, error) {
	joinedRoomIDs, err := d.Database.CurrentRoomState.SelectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
	if err != nil {
		return nil, nil, err
	}

	// Use a reasonable initial capacity
	deltas := make([]stateDelta, 0, len(joinedRoomIDs))

	// Add full states for all joined rooms
	for _, joinedRoomID := range joinedRoomIDs {
		s, stateErr := d.currentStateStreamEventsForRoom(ctx, txn, joinedRoomID, stateFilterPart)
		if stateErr != nil {
			return nil, nil, stateErr
		}
		deltas = append(deltas, stateDelta{
			membership:  gomatrixserverlib.Join,
			stateEvents: d.StreamEventsToEvents(device, s),
			roomID:      joinedRoomID,
		})
	}

	// Get all the state events ever between these two positions
	stateNeeded, eventMap, err := d.Database.OutputEvents.SelectStateInRange(ctx, txn, fromPos, toPos, stateFilterPart)
	if err != nil {
		return nil, nil, err
	}
	state, err := d.fetchStateEvents(ctx, txn, stateNeeded, eventMap)
	if err != nil {
		return nil, nil, err
	}

	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			if membership := getMembershipFromEvent(&ev.HeaderedEvent, userID); membership != "" {
				if membership != gomatrixserverlib.Join { // We've already added full state for all joined rooms above.
					deltas = append(deltas, stateDelta{
						membership:    membership,
						membershipPos: ev.StreamPosition,
						stateEvents:   d.StreamEventsToEvents(device, stateStreamEvents),
						roomID:        roomID,
					})
				}

				break
			}
		}
	}

	return deltas, joinedRoomIDs, nil
}

func (d *SyncServerDatasource) currentStateStreamEventsForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilterPart *gomatrixserverlib.StateFilter,
) ([]types.StreamEvent, error) {
	allState, err := d.Database.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, stateFilterPart)
	if err != nil {
		return nil, err
	}
	s := make([]types.StreamEvent, len(allState))
	for i := 0; i < len(s); i++ {
		s[i] = types.StreamEvent{HeaderedEvent: allState[i], StreamPosition: 0}
	}
	return s, nil
}

// StreamEventsToEvents converts streamEvent to Event. If device is non-nil and
// matches the streamevent.transactionID device then the transaction ID gets
// added to the unsigned section of the output event.
func (d *SyncServerDatasource) StreamEventsToEvents(device *authtypes.Device, in []types.StreamEvent) []gomatrixserverlib.HeaderedEvent {
	out := make([]gomatrixserverlib.HeaderedEvent, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[i].HeaderedEvent
		if device != nil && in[i].TransactionID != nil {
			if device.UserID == in[i].Sender() && device.SessionID == in[i].TransactionID.SessionID {
				err := out[i].SetUnsignedField(
					"transaction_id", in[i].TransactionID.TransactionID,
				)
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"event_id": out[i].EventID(),
					}).WithError(err).Warnf("Failed to add transaction ID to event")
				}
			}
		}
	}
	return out
}

// There may be some overlap where events in stateEvents are already in recentEvents, so filter
// them out so we don't include them twice in the /sync response. They should be in recentEvents
// only, so clients get to the correct state once they have rolled forward.
func removeDuplicates(stateEvents, recentEvents []gomatrixserverlib.HeaderedEvent) []gomatrixserverlib.HeaderedEvent {
	for _, recentEv := range recentEvents {
		if recentEv.StateKey() == nil {
			continue // not a state event
		}
		// TODO: This is a linear scan over all the current state events in this room. This will
		//       be slow for big rooms. We should instead sort the state events by event ID  (ORDER BY)
		//       then do a binary search to find matching events, similar to what roomserver does.
		for j := 0; j < len(stateEvents); j++ {
			if stateEvents[j].EventID() == recentEv.EventID() {
				// overwrite the element to remove with the last element then pop the last element.
				// This is orders of magnitude faster than re-slicing, but doesn't preserve ordering
				// (we don't care about the order of stateEvents)
				stateEvents[j] = stateEvents[len(stateEvents)-1]
				stateEvents = stateEvents[:len(stateEvents)-1]
				break // there shouldn't be multiple events with the same event ID
			}
		}
	}
	return stateEvents
}

// getMembershipFromEvent returns the value of content.membership iff the event is a state event
// with type 'm.room.member' and state_key of userID. Otherwise, an empty string is returned.
func getMembershipFromEvent(ev *gomatrixserverlib.HeaderedEvent, userID string) string {
	if ev.Type() == "m.room.member" && ev.StateKeyEquals(userID) {
		membership, err := ev.Membership()
		if err != nil {
			return ""
		}
		return membership
	}
	return ""
}
