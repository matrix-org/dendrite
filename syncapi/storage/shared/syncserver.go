package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// Database is a temporary struct until we have made syncserver.go the same for both pq/sqlite
// For now this contains the shared functions
type Database struct {
	DB                  *sql.DB
	Invites             tables.Invites
	AccountData         tables.AccountData
	OutputEvents        tables.Events
	Topology            tables.Topology
	CurrentRoomState    tables.CurrentRoomState
	BackwardExtremities tables.BackwardsExtremities
	EDUCache            *cache.EDUCache
}

// Events lookups a list of event by their event ID.
// Returns a list of events matching the requested IDs found in the database.
// If an event is not found in the database then it will be omitted from the list.
// Returns an error if there was a problem talking with the database.
// Does not include any transaction IDs in the returned events.
func (d *Database) Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.HeaderedEvent, error) {
	streamEvents, err := d.OutputEvents.SelectEvents(ctx, nil, eventIDs)
	if err != nil {
		return nil, err
	}

	// We don't include a device here as we only include transaction IDs in
	// incremental syncs.
	return d.StreamEventsToEvents(nil, streamEvents), nil
}

// GetEventsInStreamingRange retrieves all of the events on a given ordering using the
// given extremities and limit.
func (d *Database) GetEventsInStreamingRange(
	ctx context.Context,
	from, to *types.StreamingToken,
	roomID string, limit int,
	backwardOrdering bool,
) (events []types.StreamEvent, err error) {
	r := types.Range{
		From:      from.PDUPosition(),
		To:        to.PDUPosition(),
		Backwards: backwardOrdering,
	}
	if backwardOrdering {
		// When using backward ordering, we want the most recent events first.
		if events, err = d.OutputEvents.SelectRecentEvents(
			ctx, nil, roomID, r, limit, false, false,
		); err != nil {
			return
		}
	} else {
		// When using forward ordering, we want the least recent events first.
		if events, err = d.OutputEvents.SelectEarlyEvents(
			ctx, nil, roomID, r, limit,
		); err != nil {
			return
		}
	}
	return events, err
}

func (d *Database) AddTypingUser(
	userID, roomID string, expireTime *time.Time,
) types.StreamPosition {
	return types.StreamPosition(d.EDUCache.AddTypingUser(userID, roomID, expireTime))
}

func (d *Database) RemoveTypingUser(
	userID, roomID string,
) types.StreamPosition {
	return types.StreamPosition(d.EDUCache.RemoveUser(userID, roomID))
}

func (d *Database) SetTypingTimeoutCallback(fn cache.TimeoutCallbackFn) {
	d.EDUCache.SetTimeoutCallback(fn)
}

func (d *Database) AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error) {
	return d.CurrentRoomState.SelectJoinedUsers(ctx)
}

func (d *Database) GetStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.HeaderedEvent, error) {
	return d.CurrentRoomState.SelectStateEvent(ctx, roomID, evType, stateKey)
}

func (d *Database) GetStateEventsForRoom(
	ctx context.Context, roomID string, stateFilter *gomatrixserverlib.StateFilter,
) (stateEvents []gomatrixserverlib.HeaderedEvent, err error) {
	err = internal.WithTransaction(d.DB, func(txn *sql.Tx) error {
		stateEvents, err = d.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, stateFilter)
		return err
	})
	return
}

func (d *Database) SyncStreamPosition(ctx context.Context) (types.StreamPosition, error) {
	var maxID int64
	var err error
	err = internal.WithTransaction(d.DB, func(txn *sql.Tx) error {
		maxID, err = d.OutputEvents.SelectMaxEventID(ctx, txn)
		if err != nil {
			return err
		}
		var maxAccountDataID int64
		maxAccountDataID, err = d.AccountData.SelectMaxAccountDataID(ctx, txn)
		if err != nil {
			return err
		}
		if maxAccountDataID > maxID {
			maxID = maxAccountDataID
		}
		var maxInviteID int64
		maxInviteID, err = d.Invites.SelectMaxInviteID(ctx, txn)
		if err != nil {
			return err
		}
		if maxInviteID > maxID {
			maxID = maxInviteID
		}
		return nil
	})
	return types.StreamPosition(maxID), err
}

// AddInviteEvent stores a new invite event for a user.
// If the invite was successfully stored this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *Database) AddInviteEvent(
	ctx context.Context, inviteEvent gomatrixserverlib.HeaderedEvent,
) (sp types.StreamPosition, err error) {
	err = internal.WithTransaction(d.DB, func(txn *sql.Tx) error {
		sp, err = d.Invites.InsertInviteEvent(ctx, txn, inviteEvent)
		return err
	})
	return
}

// RetireInviteEvent removes an old invite event from the database.
// Returns an error if there was a problem communicating with the database.
func (d *Database) RetireInviteEvent(
	ctx context.Context, inviteEventID string,
) error {
	// TODO: Record that invite has been retired in a stream so that we can
	// notify the user in an incremental sync.
	err := d.Invites.DeleteInviteEvent(ctx, inviteEventID)
	return err
}

// GetAccountDataInRange returns all account data for a given user inserted or
// updated between two given positions
// Returns a map following the format data[roomID] = []dataTypes
// If no data is retrieved, returns an empty map
// If there was an issue with the retrieval, returns an error
func (d *Database) GetAccountDataInRange(
	ctx context.Context, userID string, r types.Range,
	accountDataFilterPart *gomatrixserverlib.EventFilter,
) (map[string][]string, error) {
	return d.AccountData.SelectAccountDataInRange(ctx, userID, r, accountDataFilterPart)
}

// UpsertAccountData keeps track of new or updated account data, by saving the type
// of the new/updated data, and the user ID and room ID the data is related to (empty)
// room ID means the data isn't specific to any room)
// If no data with the given type, user ID and room ID exists in the database,
// creates a new row, else update the existing one
// Returns an error if there was an issue with the upsert
func (d *Database) UpsertAccountData(
	ctx context.Context, userID, roomID, dataType string,
) (sp types.StreamPosition, err error) {
	err = internal.WithTransaction(d.DB, func(txn *sql.Tx) error {
		sp, err = d.AccountData.InsertAccountData(ctx, txn, userID, roomID, dataType)
		return err
	})
	return
}

func (d *Database) StreamEventsToEvents(device *authtypes.Device, in []types.StreamEvent) []gomatrixserverlib.HeaderedEvent {
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

// handleBackwardExtremities adds this event as a backwards extremity if and only if we do not have all of
// the events listed in the event's 'prev_events'. This function also updates the backwards extremities table
// to account for the fact that the given event is no longer a backwards extremity, but may be marked as such.
func (d *Database) handleBackwardExtremities(ctx context.Context, txn *sql.Tx, ev *gomatrixserverlib.HeaderedEvent) error {
	if err := d.BackwardExtremities.DeleteBackwardExtremity(ctx, txn, ev.RoomID(), ev.EventID()); err != nil {
		return err
	}

	// Check if we have all of the event's previous events. If an event is
	// missing, add it to the room's backward extremities.
	prevEvents, err := d.OutputEvents.SelectEvents(ctx, txn, ev.PrevEventIDs())
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
			if err = d.BackwardExtremities.InsertsBackwardExtremity(ctx, txn, ev.RoomID(), ev.EventID(), eID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *Database) WriteEvent(
	ctx context.Context,
	ev *gomatrixserverlib.HeaderedEvent,
	addStateEvents []gomatrixserverlib.HeaderedEvent,
	addStateEventIDs, removeStateEventIDs []string,
	transactionID *api.TransactionID, excludeFromSync bool,
) (pduPosition types.StreamPosition, returnErr error) {
	returnErr = internal.WithTransaction(d.DB, func(txn *sql.Tx) error {
		var err error
		pos, err := d.OutputEvents.InsertEvent(
			ctx, txn, ev, addStateEventIDs, removeStateEventIDs, transactionID, excludeFromSync,
		)
		if err != nil {
			return err
		}
		pduPosition = pos

		if err = d.Topology.InsertEventInTopology(ctx, txn, ev, pos); err != nil {
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

func (d *Database) updateRoomState(
	ctx context.Context, txn *sql.Tx,
	removedEventIDs []string,
	addedEvents []gomatrixserverlib.HeaderedEvent,
	pduPosition types.StreamPosition,
) error {
	// remove first, then add, as we do not ever delete state, but do replace state which is a remove followed by an add.
	for _, eventID := range removedEventIDs {
		if err := d.CurrentRoomState.DeleteRoomStateByEventID(ctx, txn, eventID); err != nil {
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

		if err := d.CurrentRoomState.UpsertRoomState(ctx, txn, event, membership, pduPosition); err != nil {
			return err
		}
	}

	return nil
}

func (d *Database) GetEventsInTopologicalRange(
	ctx context.Context,
	from, to *types.TopologyToken,
	roomID string, limit int,
	backwardOrdering bool,
) (events []types.StreamEvent, err error) {
	var minDepth, maxDepth, maxStreamPosForMaxDepth types.StreamPosition
	if backwardOrdering {
		// Backward ordering means the 'from' token has a higher depth than the 'to' token
		minDepth = to.Depth()
		maxDepth = from.Depth()
		// for cases where we have say 5 events with the same depth, the TopologyToken needs to
		// know which of the 5 the client has seen. This is done by using the PDU position.
		// Events with the same maxDepth but less than this PDU position will be returned.
		maxStreamPosForMaxDepth = from.PDUPosition()
	} else {
		// Forward ordering means the 'from' token has a lower depth than the 'to' token.
		minDepth = from.Depth()
		maxDepth = to.Depth()
	}

	// Select the event IDs from the defined range.
	var eIDs []string
	eIDs, err = d.Topology.SelectEventIDsInRange(
		ctx, nil, roomID, minDepth, maxDepth, maxStreamPosForMaxDepth, limit, !backwardOrdering,
	)
	if err != nil {
		return
	}

	// Retrieve the events' contents using their IDs.
	events, err = d.OutputEvents.SelectEvents(ctx, nil, eIDs)
	return
}

func (d *Database) SyncPosition(ctx context.Context) (tok types.StreamingToken, err error) {
	err = internal.WithTransaction(d.DB, func(txn *sql.Tx) error {
		pos, err := d.syncPositionTx(ctx, txn)
		if err != nil {
			return err
		}
		tok = pos
		return nil
	})
	return
}

func (d *Database) BackwardExtremitiesForRoom(
	ctx context.Context, roomID string,
) (backwardExtremities map[string][]string, err error) {
	return d.BackwardExtremities.SelectBackwardExtremitiesForRoom(ctx, roomID)
}

func (d *Database) MaxTopologicalPosition(
	ctx context.Context, roomID string,
) (types.TopologyToken, error) {
	depth, streamPos, err := d.Topology.SelectMaxPositionInTopology(ctx, nil, roomID)
	if err != nil {
		return types.NewTopologyToken(0, 0), err
	}
	return types.NewTopologyToken(depth, streamPos), nil
}

func (d *Database) EventPositionInTopology(
	ctx context.Context, eventID string,
) (types.TopologyToken, error) {
	depth, stream, err := d.Topology.SelectPositionInTopology(ctx, nil, eventID)
	if err != nil {
		return types.NewTopologyToken(0, 0), err
	}
	return types.NewTopologyToken(depth, stream), nil
}

func (d *Database) syncPositionTx(
	ctx context.Context, txn *sql.Tx,
) (sp types.StreamingToken, err error) {

	maxEventID, err := d.OutputEvents.SelectMaxEventID(ctx, txn)
	if err != nil {
		return sp, err
	}
	maxAccountDataID, err := d.AccountData.SelectMaxAccountDataID(ctx, txn)
	if err != nil {
		return sp, err
	}
	if maxAccountDataID > maxEventID {
		maxEventID = maxAccountDataID
	}
	maxInviteID, err := d.Invites.SelectMaxInviteID(ctx, txn)
	if err != nil {
		return sp, err
	}
	if maxInviteID > maxEventID {
		maxEventID = maxInviteID
	}
	sp = types.NewStreamToken(types.StreamPosition(maxEventID), types.StreamPosition(d.EDUCache.GetLatestSyncPosition()))
	return
}

// addPDUDeltaToResponse adds all PDU deltas to a sync response.
// IDs of all rooms the user joined are returned so EDU deltas can be added for them.
func (d *Database) addPDUDeltaToResponse(
	ctx context.Context,
	device authtypes.Device,
	r types.Range,
	numRecentEventsPerRoom int,
	wantFullState bool,
	res *types.Response,
) (joinedRoomIDs []string, err error) {
	txn, err := d.DB.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return nil, err
	}
	var succeeded bool
	defer func() {
		txerr := internal.EndTransaction(txn, &succeeded)
		if err == nil && txerr != nil {
			err = txerr
		}
	}()

	stateFilter := gomatrixserverlib.DefaultStateFilter() // TODO: use filter provided in request

	// Work out which rooms to return in the response. This is done by getting not only the currently
	// joined rooms, but also which rooms have membership transitions for this user between the 2 PDU stream positions.
	// This works out what the 'state' key should be for each room as well as which membership block
	// to put the room into.
	var deltas []stateDelta
	if !wantFullState {
		deltas, joinedRoomIDs, err = d.getStateDeltas(
			ctx, &device, txn, r, device.UserID, &stateFilter,
		)
	} else {
		deltas, joinedRoomIDs, err = d.getStateDeltasForFullStateSync(
			ctx, &device, txn, r, device.UserID, &stateFilter,
		)
	}
	if err != nil {
		return nil, err
	}

	for _, delta := range deltas {
		err = d.addRoomDeltaToResponse(ctx, &device, txn, r, delta, numRecentEventsPerRoom, res)
		if err != nil {
			return nil, err
		}
	}

	// TODO: This should be done in getStateDeltas
	if err = d.addInvitesToResponse(ctx, txn, device.UserID, r, res); err != nil {
		return nil, err
	}

	succeeded = true
	return joinedRoomIDs, nil
}

// addTypingDeltaToResponse adds all typing notifications to a sync response
// since the specified position.
func (d *Database) addTypingDeltaToResponse(
	since types.StreamingToken,
	joinedRoomIDs []string,
	res *types.Response,
) error {
	var jr types.JoinResponse
	var ok bool
	var err error
	for _, roomID := range joinedRoomIDs {
		if typingUsers, updated := d.EDUCache.GetTypingUsersIfUpdatedAfter(
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
func (d *Database) addEDUDeltaToResponse(
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

func (d *Database) IncrementalSync(
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
	if fromPos.PDUPosition() != toPos.PDUPosition() || wantFullState {
		r := types.Range{
			From: fromPos.PDUPosition(),
			To:   toPos.PDUPosition(),
		}
		joinedRoomIDs, err = d.addPDUDeltaToResponse(
			ctx, device, r, numRecentEventsPerRoom, wantFullState, res,
		)
	} else {
		joinedRoomIDs, err = d.CurrentRoomState.SelectRoomIDsWithMembership(
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
func (d *Database) getResponseWithPDUsForCompleteSync(
	ctx context.Context,
	userID string,
	numRecentEventsPerRoom int,
) (
	res *types.Response,
	toPos types.StreamingToken,
	joinedRoomIDs []string,
	err error,
) {
	// This needs to be all done in a transaction as we need to do multiple SELECTs, and we need to have
	// a consistent view of the database throughout. This includes extracting the sync position.
	// This does have the unfortunate side-effect that all the matrixy logic resides in this function,
	// but it's better to not hide the fact that this is being done in a transaction.
	txn, err := d.DB.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return
	}
	var succeeded bool
	defer func() {
		txerr := internal.EndTransaction(txn, &succeeded)
		if err == nil && txerr != nil {
			err = txerr
		}
	}()

	// Get the current sync position which we will base the sync response on.
	toPos, err = d.syncPositionTx(ctx, txn)
	if err != nil {
		return
	}
	r := types.Range{
		From: 0,
		To:   toPos.PDUPosition(),
	}

	res = types.NewResponse(toPos)

	// Extract room state and recent events for all rooms the user is joined to.
	joinedRoomIDs, err = d.CurrentRoomState.SelectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
	if err != nil {
		return
	}

	stateFilter := gomatrixserverlib.DefaultStateFilter() // TODO: use filter provided in request

	// Build up a /sync response. Add joined rooms.
	for _, roomID := range joinedRoomIDs {
		var stateEvents []gomatrixserverlib.HeaderedEvent
		stateEvents, err = d.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, &stateFilter)
		if err != nil {
			return
		}
		// TODO: When filters are added, we may need to call this multiple times to get enough events.
		//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
		var recentStreamEvents []types.StreamEvent
		recentStreamEvents, err = d.OutputEvents.SelectRecentEvents(
			ctx, txn, roomID, r, numRecentEventsPerRoom, true, true,
		)
		if err != nil {
			return
		}

		// Retrieve the backward topology position, i.e. the position of the
		// oldest event in the room's topology.
		var prevBatchStr string
		if len(recentStreamEvents) > 0 {
			var backwardTopologyPos, backwardStreamPos types.StreamPosition
			backwardTopologyPos, backwardStreamPos, err = d.Topology.SelectPositionInTopology(ctx, txn, recentStreamEvents[0].EventID())
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

	if err = d.addInvitesToResponse(ctx, txn, userID, r, res); err != nil {
		return
	}

	succeeded = true
	return res, toPos, joinedRoomIDs, err
}

func (d *Database) CompleteSync(
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
		types.NewStreamToken(0, 0), toPos, joinedRoomIDs, res,
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

func (d *Database) addInvitesToResponse(
	ctx context.Context, txn *sql.Tx,
	userID string,
	r types.Range,
	res *types.Response,
) error {
	invites, err := d.Invites.SelectInviteEventsInRange(
		ctx, txn, userID, r,
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
func (d *Database) getBackwardTopologyPos(
	ctx context.Context, txn *sql.Tx,
	events []types.StreamEvent,
) (types.TopologyToken, error) {
	zeroToken := types.NewTopologyToken(0, 0)
	if len(events) == 0 {
		return zeroToken, nil
	}
	pos, spos, err := d.Topology.SelectPositionInTopology(ctx, txn, events[0].EventID())
	if err != nil {
		return zeroToken, err
	}
	tok := types.NewTopologyToken(pos, spos)
	tok.Decrement()
	return tok, nil
}

// addRoomDeltaToResponse adds a room state delta to a sync response
func (d *Database) addRoomDeltaToResponse(
	ctx context.Context,
	device *authtypes.Device,
	txn *sql.Tx,
	r types.Range,
	delta stateDelta,
	numRecentEventsPerRoom int,
	res *types.Response,
) error {
	if delta.membershipPos > 0 && delta.membership == gomatrixserverlib.Leave {
		// make sure we don't leak recent events after the leave event.
		// TODO: History visibility makes this somewhat complex to handle correctly. For example:
		// TODO: This doesn't work for join -> leave in a single /sync request (see events prior to join).
		// TODO: This will fail on join -> leave -> sensitive msg -> join -> leave
		//       in a single /sync request
		// This is all "okay" assuming history_visibility == "shared" which it is by default.
		r.To = delta.membershipPos
	}
	recentStreamEvents, err := d.OutputEvents.SelectRecentEvents(
		ctx, txn, delta.roomID, r,
		numRecentEventsPerRoom, true, true,
	)
	if err != nil {
		return err
	}
	recentEvents := d.StreamEventsToEvents(device, recentStreamEvents)
	delta.stateEvents = removeDuplicates(delta.stateEvents, recentEvents) // roll back
	prevBatch, err := d.getBackwardTopologyPos(ctx, txn, recentStreamEvents)
	if err != nil {
		return err
	}

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
func (d *Database) fetchStateEvents(
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

func (d *Database) fetchMissingStateEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	// Fetch from the events table first so we pick up the stream ID for the
	// event.
	events, err := d.OutputEvents.SelectEvents(ctx, txn, eventIDs)
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
	stateEvents, err := d.CurrentRoomState.SelectEventsWithEventIDs(ctx, txn, missing)

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
func (d *Database) getStateDeltas(
	ctx context.Context, device *authtypes.Device, txn *sql.Tx,
	r types.Range, userID string,
	stateFilter *gomatrixserverlib.StateFilter,
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
	stateNeeded, eventMap, err := d.OutputEvents.SelectStateInRange(ctx, txn, r, stateFilter)
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
					s, err = d.currentStateStreamEventsForRoom(ctx, txn, roomID, stateFilter)
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
	joinedRoomIDs, err := d.CurrentRoomState.SelectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
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
func (d *Database) getStateDeltasForFullStateSync(
	ctx context.Context, device *authtypes.Device, txn *sql.Tx,
	r types.Range, userID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]stateDelta, []string, error) {
	joinedRoomIDs, err := d.CurrentRoomState.SelectRoomIDsWithMembership(ctx, txn, userID, gomatrixserverlib.Join)
	if err != nil {
		return nil, nil, err
	}

	// Use a reasonable initial capacity
	deltas := make([]stateDelta, 0, len(joinedRoomIDs))

	// Add full states for all joined rooms
	for _, joinedRoomID := range joinedRoomIDs {
		s, stateErr := d.currentStateStreamEventsForRoom(ctx, txn, joinedRoomID, stateFilter)
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
	stateNeeded, eventMap, err := d.OutputEvents.SelectStateInRange(ctx, txn, r, stateFilter)
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

func (d *Database) currentStateStreamEventsForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]types.StreamEvent, error) {
	allState, err := d.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, stateFilter)
	if err != nil {
		return nil, err
	}
	s := make([]types.StreamEvent, len(allState))
	for i := 0; i < len(s); i++ {
		s[i] = types.StreamEvent{HeaderedEvent: allState[i], StreamPosition: 0}
	}
	return s, nil
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

type stateDelta struct {
	roomID      string
	stateEvents []gomatrixserverlib.HeaderedEvent
	membership  string
	// The PDU stream position of the latest membership event for this user, if applicable.
	// Can be 0 if there is no membership event in this delta.
	membershipPos types.StreamPosition
}
