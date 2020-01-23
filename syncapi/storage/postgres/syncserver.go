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

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/typingserver/cache"
	"github.com/matrix-org/gomatrixserverlib"
)

type stateDelta struct {
	roomID      string
	stateEvents []gomatrixserverlib.Event
	membership  string
	// The PDU stream position of the latest membership event for this user, if applicable.
	// Can be 0 if there is no membership event in this delta.
	membershipPos types.StreamPosition
}

// SyncServerDatasource represents a sync server datasource which manages
// both the database for PDUs and caches for EDUs.
type SyncServerDatasource struct {
	db *sql.DB
	common.PartitionOffsetStatements
	accountData         accountDataStatements
	events              outputRoomEventsStatements
	roomstate           currentRoomStateStatements
	invites             inviteEventsStatements
	typingCache         *cache.TypingCache
	topology            outputRoomEventsTopologyStatements
	backwardExtremities backwardExtremitiesStatements
}

// NewSyncServerDatasource creates a new sync server database
func NewSyncServerDatasource(dbDataSourceName string) (*SyncServerDatasource, error) {
	var d SyncServerDatasource
	var err error
	if d.db, err = sql.Open("postgres", dbDataSourceName); err != nil {
		return nil, err
	}
	if err = d.PartitionOffsetStatements.Prepare(d.db, "syncapi"); err != nil {
		return nil, err
	}
	if err = d.accountData.prepare(d.db); err != nil {
		return nil, err
	}
	if err = d.events.prepare(d.db); err != nil {
		return nil, err
	}
	if err := d.roomstate.prepare(d.db); err != nil {
		return nil, err
	}
	if err := d.invites.prepare(d.db); err != nil {
		return nil, err
	}
	if err := d.topology.prepare(d.db); err != nil {
		return nil, err
	}
	if err := d.backwardExtremities.prepare(d.db); err != nil {
		return nil, err
	}
	d.typingCache = cache.NewTypingCache()
	return &d, nil
}

// AllJoinedUsersInRooms returns a map of room ID to a list of all joined user IDs.
func (d *SyncServerDatasource) AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error) {
	return d.roomstate.selectJoinedUsers(ctx)
}

// Events lookups a list of event by their event ID.
// Returns a list of events matching the requested IDs found in the database.
// If an event is not found in the database then it will be omitted from the list.
// Returns an error if there was a problem talking with the database.
// Does not include any transaction IDs in the returned events.
func (d *SyncServerDatasource) Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.Event, error) {
	streamEvents, err := d.events.selectEvents(ctx, nil, eventIDs)
	if err != nil {
		return nil, err
	}

	// We don't include a device here as we only include transaction IDs in
	// incremental syncs.
	return d.StreamEventsToEvents(nil, streamEvents), nil
}

func (d *SyncServerDatasource) handleBackwardExtremities(ctx context.Context, ev *gomatrixserverlib.Event) error {
	// If the event is already known as a backward extremity, don't consider
	// it as such anymore now that we have it.
	isBackwardExtremity, err := d.backwardExtremities.isBackwardExtremity(ctx, ev.RoomID(), ev.EventID())
	if err != nil {
		return err
	}
	if isBackwardExtremity {
		if err = d.backwardExtremities.deleteBackwardExtremity(ctx, ev.RoomID(), ev.EventID()); err != nil {
			return err
		}
	}

	// Check if we have all of the event's previous events. If an event is
	// missing, add it to the room's backward extremities.
	prevEvents, err := d.events.selectEvents(ctx, nil, ev.PrevEventIDs())
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
			if err = d.backwardExtremities.insertsBackwardExtremity(ctx, ev.RoomID(), ev.EventID()); err != nil {
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
	ev *gomatrixserverlib.Event,
	addStateEvents []gomatrixserverlib.Event,
	addStateEventIDs, removeStateEventIDs []string,
	transactionID *api.TransactionID, excludeFromSync bool,
) (pduPosition types.StreamPosition, returnErr error) {
	returnErr = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		var err error
		pos, err := d.events.insertEvent(
			ctx, txn, ev, addStateEventIDs, removeStateEventIDs, transactionID, excludeFromSync,
		)
		if err != nil {
			return err
		}
		pduPosition = pos

		if err = d.topology.insertEventInTopology(ctx, ev); err != nil {
			return err
		}

		if err = d.handleBackwardExtremities(ctx, ev); err != nil {
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
	addedEvents []gomatrixserverlib.Event,
	pduPosition types.StreamPosition,
) error {
	// remove first, then add, as we do not ever delete state, but do replace state which is a remove followed by an add.
	for _, eventID := range removedEventIDs {
		if err := d.roomstate.deleteRoomStateByEventID(ctx, txn, eventID); err != nil {
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
		if err := d.roomstate.upsertRoomState(ctx, txn, event, membership, pduPosition); err != nil {
			return err
		}
	}

	return nil
}

// GetStateEvent returns the Matrix state event of a given type for a given room with a given state key
// If no event could be found, returns nil
// If there was an issue during the retrieval, returns an error
func (d *SyncServerDatasource) GetStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.Event, error) {
	return d.roomstate.selectStateEvent(ctx, roomID, evType, stateKey)
}

// GetStateEventsForRoom fetches the state events for a given room.
// Returns an empty slice if no state events could be found for this room.
// Returns an error if there was an issue with the retrieval.
func (d *SyncServerDatasource) GetStateEventsForRoom(
	ctx context.Context, roomID string, stateFilterPart *gomatrix.FilterPart,
) (stateEvents []gomatrixserverlib.Event, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		stateEvents, err = d.roomstate.selectCurrentState(ctx, txn, roomID, stateFilterPart)
		return err
	})
	return
}

// GetEventsInRange retrieves all of the events on a given ordering using the
// given extremities and limit.
func (d *SyncServerDatasource) GetEventsInRange(
	ctx context.Context,
	from, to *types.PaginationToken,
	roomID string, limit int,
	backwardOrdering bool,
) (events []types.StreamEvent, err error) {
	// If the pagination token's type is types.PaginationTokenTypeTopology, the
	// events must be retrieved from the rooms' topology table rather than the
	// table contaning the syncapi server's whole stream of events.
	if from.Type == types.PaginationTokenTypeTopology {
		// Determine the backward and forward limit, i.e. the upper and lower
		// limits to the selection in the room's topology, from the direction.
		var backwardLimit, forwardLimit types.StreamPosition
		if backwardOrdering {
			// Backward ordering is antichronological (latest event to oldest
			// one).
			backwardLimit = to.PDUPosition
			forwardLimit = from.PDUPosition
		} else {
			// Forward ordering is chronological (oldest event to latest one).
			backwardLimit = from.PDUPosition
			forwardLimit = to.PDUPosition
		}

		// Select the event IDs from the defined range.
		var eIDs []string
		eIDs, err = d.topology.selectEventIDsInRange(
			ctx, roomID, backwardLimit, forwardLimit, limit, !backwardOrdering,
		)
		if err != nil {
			return
		}

		// Retrieve the events' contents using their IDs.
		events, err = d.events.selectEvents(ctx, nil, eIDs)
		return
	}

	// If the pagination token's type is types.PaginationTokenTypeStream, the
	// events must be retrieved from the table contaning the syncapi server's
	// whole stream of events.

	if backwardOrdering {
		// When using backward ordering, we want the most recent events first.
		if events, err = d.events.selectRecentEvents(
			ctx, nil, roomID, to.PDUPosition, from.PDUPosition, limit, false, false,
		); err != nil {
			return
		}
	} else {
		// When using forward ordering, we want the least recent events first.
		if events, err = d.events.selectEarlyEvents(
			ctx, nil, roomID, from.PDUPosition, to.PDUPosition, limit,
		); err != nil {
			return
		}
	}

	return
}

// SyncPosition returns the latest positions for syncing.
func (d *SyncServerDatasource) SyncPosition(ctx context.Context) (types.PaginationToken, error) {
	return d.syncPositionTx(ctx, nil)
}

// BackwardExtremitiesForRoom returns the event IDs of all of the backward
// extremities we know of for a given room.
func (d *SyncServerDatasource) BackwardExtremitiesForRoom(
	ctx context.Context, roomID string,
) (backwardExtremities []string, err error) {
	return d.backwardExtremities.selectBackwardExtremitiesForRoom(ctx, roomID)
}

// MaxTopologicalPosition returns the highest topological position for a given
// room.
func (d *SyncServerDatasource) MaxTopologicalPosition(
	ctx context.Context, roomID string,
) (types.StreamPosition, error) {
	return d.topology.selectMaxPositionInTopology(ctx, roomID)
}

// EventsAtTopologicalPosition returns all of the events matching a given
// position in the topology of a given room.
func (d *SyncServerDatasource) EventsAtTopologicalPosition(
	ctx context.Context, roomID string, pos types.StreamPosition,
) ([]types.StreamEvent, error) {
	eIDs, err := d.topology.selectEventIDsFromPosition(ctx, roomID, pos)
	if err != nil {
		return nil, err
	}

	return d.events.selectEvents(ctx, nil, eIDs)
}

func (d *SyncServerDatasource) EventPositionInTopology(
	ctx context.Context, eventID string,
) (types.StreamPosition, error) {
	return d.topology.selectPositionInTopology(ctx, eventID)
}

// SyncStreamPosition returns the latest position in the sync stream. Returns 0 if there are no events yet.
func (d *SyncServerDatasource) SyncStreamPosition(ctx context.Context) (types.StreamPosition, error) {
	return d.syncStreamPositionTx(ctx, nil)
}

func (d *SyncServerDatasource) syncStreamPositionTx(
	ctx context.Context, txn *sql.Tx,
) (types.StreamPosition, error) {
	maxID, err := d.events.selectMaxEventID(ctx, txn)
	if err != nil {
		return 0, err
	}
	maxAccountDataID, err := d.accountData.selectMaxAccountDataID(ctx, txn)
	if err != nil {
		return 0, err
	}
	if maxAccountDataID > maxID {
		maxID = maxAccountDataID
	}
	maxInviteID, err := d.invites.selectMaxInviteID(ctx, txn)
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
) (sp types.PaginationToken, err error) {

	maxEventID, err := d.events.selectMaxEventID(ctx, txn)
	if err != nil {
		return sp, err
	}
	maxAccountDataID, err := d.accountData.selectMaxAccountDataID(ctx, txn)
	if err != nil {
		return sp, err
	}
	if maxAccountDataID > maxEventID {
		maxEventID = maxAccountDataID
	}
	maxInviteID, err := d.invites.selectMaxInviteID(ctx, txn)
	if err != nil {
		return sp, err
	}
	if maxInviteID > maxEventID {
		maxEventID = maxInviteID
	}
	sp.PDUPosition = types.StreamPosition(maxEventID)
	sp.EDUTypingPosition = types.StreamPosition(d.typingCache.GetLatestSyncPosition())
	return
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
) ([]string, error) {
	txn, err := d.db.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return nil, err
	}
	var succeeded bool
	defer common.EndTransaction(txn, &succeeded)

	stateFilterPart := gomatrix.DefaultFilterPart() // TODO: use filter provided in request

	// Work out which rooms to return in the response. This is done by getting not only the currently
	// joined rooms, but also which rooms have membership transitions for this user between the 2 PDU stream positions.
	// This works out what the 'state' key should be for each room as well as which membership block
	// to put the room into.
	var deltas []stateDelta
	var joinedRoomIDs []string
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
	since types.PaginationToken,
	joinedRoomIDs []string,
	res *types.Response,
) error {
	var jr types.JoinResponse
	var ok bool
	var err error
	for _, roomID := range joinedRoomIDs {
		if typingUsers, updated := d.typingCache.GetTypingUsersIfUpdatedAfter(
			roomID, int64(since.EDUTypingPosition),
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
	fromPos, toPos types.PaginationToken,
	joinedRoomIDs []string,
	res *types.Response,
) (err error) {

	if fromPos.EDUTypingPosition != toPos.EDUTypingPosition {
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
	fromPos, toPos types.PaginationToken,
	numRecentEventsPerRoom int,
	wantFullState bool,
) (*types.Response, error) {
	nextBatchPos := fromPos.WithUpdates(toPos)
	res := types.NewResponse(nextBatchPos)

	var joinedRoomIDs []string
	var err error
	if fromPos.PDUPosition != toPos.PDUPosition || wantFullState {
		joinedRoomIDs, err = d.addPDUDeltaToResponse(
			ctx, device, fromPos.PDUPosition, toPos.PDUPosition, numRecentEventsPerRoom, wantFullState, res,
		)
	} else {
		joinedRoomIDs, err = d.roomstate.selectRoomIDsWithMembership(
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
	toPos types.PaginationToken,
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
	defer common.EndTransaction(txn, &succeeded)

	// Get the current sync position which we will base the sync response on.
	toPos, err = d.syncPositionTx(ctx, txn)
	if err != nil {
		return
	}

	res = types.NewResponse(toPos)

	// Extract room state and recent events for all rooms the user is joined to.
	joinedRoomIDs, err = d.roomstate.selectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
	if err != nil {
		return
	}

	stateFilterPart := gomatrix.DefaultFilterPart() // TODO: use filter provided in request

	// Build up a /sync response. Add joined rooms.
	for _, roomID := range joinedRoomIDs {
		var stateEvents []gomatrixserverlib.Event
		stateEvents, err = d.roomstate.selectCurrentState(ctx, txn, roomID, &stateFilterPart)
		if err != nil {
			return
		}
		// TODO: When filters are added, we may need to call this multiple times to get enough events.
		//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
		var recentStreamEvents []types.StreamEvent
		recentStreamEvents, err = d.events.selectRecentEvents(
			ctx, txn, roomID, types.StreamPosition(0), toPos.PDUPosition,
			numRecentEventsPerRoom, true, true,
			//ctx, txn, roomID, 0, toPos.PDUPosition, numRecentEventsPerRoom,
		)
		if err != nil {
			return
		}

		// Retrieve the backward topology position, i.e. the position of the
		// oldest event in the room's topology.
		var backwardTopologyPos types.StreamPosition
		backwardTopologyPos, err = d.topology.selectPositionInTopology(ctx, recentStreamEvents[0].EventID())
		if err != nil {
			return nil, types.PaginationToken{}, []string{}, err
		}
		if backwardTopologyPos-1 <= 0 {
			backwardTopologyPos = types.StreamPosition(1)
		} else {
			backwardTopologyPos = backwardTopologyPos - 1
		}

		// We don't include a device here as we don't need to send down
		// transaction IDs for complete syncs
		recentEvents := d.StreamEventsToEvents(nil, recentStreamEvents)
		stateEvents = removeDuplicates(stateEvents, recentEvents)
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = types.NewPaginationTokenFromTypeAndPosition(
			types.PaginationTokenTypeTopology, backwardTopologyPos, 0,
		).String()
		jr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = true
		jr.State.Events = gomatrixserverlib.ToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[roomID] = *jr
	}

	if err = d.addInvitesToResponse(ctx, txn, userID, 0, toPos.PDUPosition, res); err != nil {
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
		types.PaginationToken{}, toPos, joinedRoomIDs, res,
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

// GetAccountDataInRange returns all account data for a given user inserted or
// updated between two given positions
// Returns a map following the format data[roomID] = []dataTypes
// If no data is retrieved, returns an empty map
// If there was an issue with the retrieval, returns an error
func (d *SyncServerDatasource) GetAccountDataInRange(
	ctx context.Context, userID string, oldPos, newPos types.StreamPosition,
	accountDataFilterPart *gomatrix.FilterPart,
) (map[string][]string, error) {
	return d.accountData.selectAccountDataInRange(ctx, userID, oldPos, newPos, accountDataFilterPart)
}

// UpsertAccountData keeps track of new or updated account data, by saving the type
// of the new/updated data, and the user ID and room ID the data is related to (empty)
// room ID means the data isn't specific to any room)
// If no data with the given type, user ID and room ID exists in the database,
// creates a new row, else update the existing one
// Returns an error if there was an issue with the upsert
func (d *SyncServerDatasource) UpsertAccountData(
	ctx context.Context, userID, roomID, dataType string,
) (types.StreamPosition, error) {
	return d.accountData.insertAccountData(ctx, userID, roomID, dataType)
}

// AddInviteEvent stores a new invite event for a user.
// If the invite was successfully stored this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *SyncServerDatasource) AddInviteEvent(
	ctx context.Context, inviteEvent gomatrixserverlib.Event,
) (types.StreamPosition, error) {
	return d.invites.insertInviteEvent(ctx, inviteEvent)
}

// RetireInviteEvent removes an old invite event from the database.
// Returns an error if there was a problem communicating with the database.
func (d *SyncServerDatasource) RetireInviteEvent(
	ctx context.Context, inviteEventID string,
) error {
	// TODO: Record that invite has been retired in a stream so that we can
	// notify the user in an incremental sync.
	err := d.invites.deleteInviteEvent(ctx, inviteEventID)
	return err
}

func (d *SyncServerDatasource) SetTypingTimeoutCallback(fn cache.TimeoutCallbackFn) {
	d.typingCache.SetTimeoutCallback(fn)
}

// AddTypingUser adds a typing user to the typing cache.
// Returns the newly calculated sync position for typing notifications.
func (d *SyncServerDatasource) AddTypingUser(
	userID, roomID string, expireTime *time.Time,
) types.StreamPosition {
	return types.StreamPosition(d.typingCache.AddTypingUser(userID, roomID, expireTime))
}

// RemoveTypingUser removes a typing user from the typing cache.
// Returns the newly calculated sync position for typing notifications.
func (d *SyncServerDatasource) RemoveTypingUser(
	userID, roomID string,
) types.StreamPosition {
	return types.StreamPosition(d.typingCache.RemoveUser(userID, roomID))
}

func (d *SyncServerDatasource) addInvitesToResponse(
	ctx context.Context, txn *sql.Tx,
	userID string,
	fromPos, toPos types.StreamPosition,
	res *types.Response,
) error {
	invites, err := d.invites.selectInviteEventsInRange(
		ctx, txn, userID, fromPos, toPos,
	)
	if err != nil {
		return err
	}
	for roomID, inviteEvent := range invites {
		ir := types.NewInviteResponse()
		ir.InviteState.Events = gomatrixserverlib.ToClientEvents(
			[]gomatrixserverlib.Event{inviteEvent}, gomatrixserverlib.FormatSync,
		)
		// TODO: add the invite state from the invite event.
		res.Rooms.Invite[roomID] = *ir
	}
	return nil
}

// Retrieve the backward topology position, i.e. the position of the
// oldest event in the room's topology.
func (d *SyncServerDatasource) getBackwardTopologyPos(
	ctx context.Context,
	events []types.StreamEvent,
) (pos types.StreamPosition, err error) {
	if len(events) > 0 {
		pos, err = d.topology.selectPositionInTopology(ctx, events[0].EventID())
		if err != nil {
			return
		}
	}
	if pos-1 <= 0 {
		pos = types.StreamPosition(1)
	} else {
		pos = pos - 1
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
	recentStreamEvents, err := d.events.selectRecentEvents(
		ctx, txn, delta.roomID, types.StreamPosition(fromPos), types.StreamPosition(endPos),
		numRecentEventsPerRoom, true, true,
	)
	if err != nil {
		return err
	}
	recentEvents := d.StreamEventsToEvents(device, recentStreamEvents)
	delta.stateEvents = removeDuplicates(delta.stateEvents, recentEvents) // roll back

	var backwardTopologyPos types.StreamPosition
	backwardTopologyPos, err = d.getBackwardTopologyPos(ctx, recentStreamEvents)
	if err != nil {
		return err
	}

	switch delta.membership {
	case gomatrixserverlib.Join:
		jr := types.NewJoinResponse()

		jr.Timeline.PrevBatch = types.NewPaginationTokenFromTypeAndPosition(
			types.PaginationTokenTypeTopology, backwardTopologyPos, 0,
		).String()
		jr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		jr.State.Events = gomatrixserverlib.ToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[delta.roomID] = *jr
	case gomatrixserverlib.Leave:
		fallthrough // transitions to leave are the same as ban
	case gomatrixserverlib.Ban:
		// TODO: recentEvents may contain events that this user is not allowed to see because they are
		//       no longer in the room.
		lr := types.NewLeaveResponse()
		lr.Timeline.PrevBatch = types.NewPaginationTokenFromTypeAndPosition(
			types.PaginationTokenTypeTopology, backwardTopologyPos, 0,
		).String()
		lr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		lr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		lr.State.Events = gomatrixserverlib.ToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
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
	events, err := d.events.selectEvents(ctx, txn, eventIDs)
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
	stateEvents, err := d.roomstate.selectEventsWithEventIDs(ctx, txn, missing)

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
	stateFilterPart *gomatrix.FilterPart,
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
	stateNeeded, eventMap, err := d.events.selectStateInRange(ctx, txn, fromPos, toPos, stateFilterPart)
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
			if membership := getMembershipFromEvent(&ev.Event, userID); membership != "" {
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
	joinedRoomIDs, err := d.roomstate.selectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
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
	stateFilterPart *gomatrix.FilterPart,
) ([]stateDelta, []string, error) {
	joinedRoomIDs, err := d.roomstate.selectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
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
	stateNeeded, eventMap, err := d.events.selectStateInRange(ctx, txn, fromPos, toPos, stateFilterPart)
	if err != nil {
		return nil, nil, err
	}
	state, err := d.fetchStateEvents(ctx, txn, stateNeeded, eventMap)
	if err != nil {
		return nil, nil, err
	}

	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			if membership := getMembershipFromEvent(&ev.Event, userID); membership != "" {
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
	stateFilterPart *gomatrix.FilterPart,
) ([]types.StreamEvent, error) {
	allState, err := d.roomstate.selectCurrentState(ctx, txn, roomID, stateFilterPart)
	if err != nil {
		return nil, err
	}
	s := make([]types.StreamEvent, len(allState))
	for i := 0; i < len(s); i++ {
		s[i] = types.StreamEvent{Event: allState[i], StreamPosition: 0}
	}
	return s, nil
}

// StreamEventsToEvents converts streamEvent to Event. If device is non-nil and
// matches the streamevent.transactionID device then the transaction ID gets
// added to the unsigned section of the output event.
func (d *SyncServerDatasource) StreamEventsToEvents(device *authtypes.Device, in []types.StreamEvent) []gomatrixserverlib.Event {
	out := make([]gomatrixserverlib.Event, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[i].Event
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
func removeDuplicates(stateEvents, recentEvents []gomatrixserverlib.Event) []gomatrixserverlib.Event {
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
func getMembershipFromEvent(ev *gomatrixserverlib.Event, userID string) string {
	if ev.Type() == "m.room.member" && ev.StateKeyEquals(userID) {
		membership, err := ev.Membership()
		if err != nil {
			return ""
		}
		return membership
	}
	return ""
}
