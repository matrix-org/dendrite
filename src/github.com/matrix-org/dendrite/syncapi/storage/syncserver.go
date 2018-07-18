// Copyright 2017 Vector Creations Ltd
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

package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/roomserver/api"

	"encoding/json"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type stateDelta struct {
	roomID      string
	stateEvents []gomatrixserverlib.Event
	membership  string
	// The stream position of the latest membership event for this user, if applicable.
	// Can be 0 if there is no membership event in this delta.
	membershipPos types.StreamPosition
}

// Same as gomatrixserverlib.Event but also has the stream position for this event.
type streamEvent struct {
	gomatrixserverlib.Event
	streamPosition types.StreamPosition
	transactionID  *api.TransactionID
}

// SyncServerDatabase represents a sync server database
type SyncServerDatabase struct {
	db *sql.DB
	common.PartitionOffsetStatements
	accountData accountDataStatements
	events      outputRoomEventsStatements
	roomstate   currentRoomStateStatements
	invites     inviteEventsStatements
	stdMsg      stdEventsStatements
}

// NewSyncServerDatabase creates a new sync server database
func NewSyncServerDatabase(dataSourceName string) (*SyncServerDatabase, error) {
	var d SyncServerDatabase
	var err error
	if d.db, err = sql.Open("postgres", dataSourceName); err != nil {
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
	if err := d.stdMsg.prepare(d.db); err != nil {
		return nil, err
	}
	return &d, nil
}

// AllJoinedUsersInRooms returns a map of room ID to a list of all joined user IDs.
func (d *SyncServerDatabase) AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error) {
	return d.roomstate.selectJoinedUsers(ctx)
}

// Events lookups a list of event by their event ID.
// Returns a list of events matching the requested IDs found in the database.
// If an event is not found in the database then it will be omitted from the list.
// Returns an error if there was a problem talking with the database.
// Does not include any transaction IDs in the returned events.
func (d *SyncServerDatabase) Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.Event, error) {
	streamEvents, err := d.events.selectEvents(ctx, nil, eventIDs)
	if err != nil {
		return nil, err
	}

	// We don't include a device here as we only include transaction IDs in
	// incremental syncs.
	return streamEventsToEvents(nil, streamEvents), nil
}

// WriteEvent into the database. It is not safe to call this function from multiple goroutines, as it would create races
// when generating the stream position for this event. Returns the sync stream position for the inserted event.
// Returns an error if there was a problem inserting this event.
func (d *SyncServerDatabase) WriteEvent(
	ctx context.Context,
	ev *gomatrixserverlib.Event,
	addStateEvents []gomatrixserverlib.Event,
	addStateEventIDs, removeStateEventIDs []string,
	transactionID *api.TransactionID,
) (streamPos types.StreamPosition, returnErr error) {
	returnErr = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		var err error
		pos, err := d.events.insertEvent(ctx, txn, ev, addStateEventIDs, removeStateEventIDs, transactionID)
		if err != nil {
			return err
		}
		streamPos = types.StreamPosition(pos)

		if len(addStateEvents) == 0 && len(removeStateEventIDs) == 0 {
			// Nothing to do, the event may have just been a message event.
			return nil
		}

		return d.updateRoomState(ctx, txn, removeStateEventIDs, addStateEvents, streamPos)
	})
	return
}

func (d *SyncServerDatabase) updateRoomState(
	ctx context.Context, txn *sql.Tx,
	removedEventIDs []string,
	addedEvents []gomatrixserverlib.Event,
	streamPos types.StreamPosition,
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
		if err := d.roomstate.upsertRoomState(ctx, txn, event, membership, int64(streamPos)); err != nil {
			return err
		}
	}

	return nil
}

// GetStateEvent returns the Matrix state event of a given type for a given room with a given state key
// If no event could be found, returns nil
// If there was an issue during the retrieval, returns an error
func (d *SyncServerDatabase) GetStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.Event, error) {
	return d.roomstate.selectStateEvent(ctx, roomID, evType, stateKey)
}

// GetStateEventsForRoom fetches the state events for a given room.
// Returns an empty slice if no state events could be found for this room.
// Returns an error if there was an issue with the retrieval.
func (d *SyncServerDatabase) GetStateEventsForRoom(
	ctx context.Context, roomID string,
) (stateEvents []gomatrixserverlib.Event, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		stateEvents, err = d.roomstate.selectCurrentState(ctx, txn, roomID)
		return err
	})
	return
}

// SyncStreamPosition returns the latest position in the sync stream. Returns 0 if there are no events yet.
func (d *SyncServerDatabase) SyncStreamPosition(ctx context.Context) (types.StreamPosition, error) {
	return d.syncStreamPositionTx(ctx, nil)
}

func (d *SyncServerDatabase) syncStreamPositionTx(
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
	maxStdID, err := d.stdMsg.selectMaxStdID(ctx, txn)
	if err != nil {
		return 0, err
	}
	if maxStdID > maxID {
		maxID = maxStdID
	}
	return types.StreamPosition(maxID), nil
}

// IncrementalSync returns all the data needed in order to create an incremental
// sync response for the given user. Events returned will include any client
// transaction IDs associated with the given device. These transaction IDs come
// from when the device sent the event via an API that included a transaction
// ID.
func (d *SyncServerDatabase) IncrementalSync(
	ctx context.Context,
	device authtypes.Device,
	fromPos, toPos types.StreamPosition,
	numRecentEventsPerRoom int,
) (*types.Response, error) {
	txn, err := d.db.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return nil, err
	}
	var succeeded bool
	defer common.EndTransaction(txn, &succeeded)

	// Work out which rooms to return in the response. This is done by getting not only the currently
	// joined rooms, but also which rooms have membership transitions for this user between the 2 stream positions.
	// This works out what the 'state' key should be for each room as well as which membership block
	// to put the room into.
	deltas, err := d.getStateDeltas(ctx, &device, txn, fromPos, toPos, device.UserID)
	if err != nil {
		return nil, err
	}

	res := types.NewResponse(toPos)
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
	return res, nil
}

// CompleteSync a complete /sync API response for the given user.
func (d *SyncServerDatabase) CompleteSync(
	ctx context.Context, userID string, numRecentEventsPerRoom int,
) (*types.Response, error) {
	// This needs to be all done in a transaction as we need to do multiple SELECTs, and we need to have
	// a consistent view of the database throughout. This includes extracting the sync stream position.
	// This does have the unfortunate side-effect that all the matrixy logic resides in this function,
	// but it's better to not hide the fact that this is being done in a transaction.
	txn, err := d.db.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return nil, err
	}
	var succeeded bool
	defer common.EndTransaction(txn, &succeeded)

	// Get the current stream position which we will base the sync response on.
	pos, err := d.syncStreamPositionTx(ctx, txn)
	if err != nil {
		return nil, err
	}

	// Extract room state and recent events for all rooms the user is joined to.
	roomIDs, err := d.roomstate.selectRoomIDsWithMembership(ctx, txn, userID, "join")
	if err != nil {
		return nil, err
	}

	// Build up a /sync response. Add joined rooms.
	res := types.NewResponse(pos)
	for _, roomID := range roomIDs {
		var stateEvents []gomatrixserverlib.Event
		stateEvents, err = d.roomstate.selectCurrentState(ctx, txn, roomID)
		if err != nil {
			return nil, err
		}
		// TODO: When filters are added, we may need to call this multiple times to get enough events.
		//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
		var recentStreamEvents []streamEvent
		recentStreamEvents, err = d.events.selectRecentEvents(
			ctx, txn, roomID, types.StreamPosition(0), pos, numRecentEventsPerRoom,
		)
		if err != nil {
			return nil, err
		}

		// We don't include a device here as we don't need to send down
		// transaction IDs for complete syncs
		recentEvents := streamEventsToEvents(nil, recentStreamEvents)

		stateEvents = removeDuplicates(stateEvents, recentEvents)
		jr := types.NewJoinResponse()
		jr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = true
		jr.State.Events = gomatrixserverlib.ToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[roomID] = *jr
	}

	if err = d.addInvitesToResponse(ctx, txn, userID, 0, pos, res); err != nil {
		return nil, err
	}

	succeeded = true
	return res, err
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
func (d *SyncServerDatabase) GetAccountDataInRange(
	ctx context.Context, userID string, oldPos, newPos types.StreamPosition,
) (map[string][]string, error) {
	return d.accountData.selectAccountDataInRange(ctx, userID, oldPos, newPos)
}

// UpsertAccountData keeps track of new or updated account data, by saving the type
// of the new/updated data, and the user ID and room ID the data is related to (empty)
// room ID means the data isn't specific to any room)
// If no data with the given type, user ID and room ID exists in the database,
// creates a new row, else update the existing one
// Returns an error if there was an issue with the upsert
func (d *SyncServerDatabase) UpsertAccountData(
	ctx context.Context, userID, roomID, dataType string,
) (types.StreamPosition, error) {
	pos, err := d.accountData.insertAccountData(ctx, userID, roomID, dataType)
	return types.StreamPosition(pos), err
}

// AddInviteEvent stores a new invite event for a user.
// If the invite was successfully stored this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *SyncServerDatabase) AddInviteEvent(
	ctx context.Context, inviteEvent gomatrixserverlib.Event,
) (types.StreamPosition, error) {
	pos, err := d.invites.insertInviteEvent(ctx, inviteEvent)
	return types.StreamPosition(pos), err
}

// RetireInviteEvent removes an old invite event from the database.
// Returns an error if there was a problem communicating with the database.
func (d *SyncServerDatabase) RetireInviteEvent(
	ctx context.Context, inviteEventID string,
) error {
	// TODO: Record that invite has been retired in a stream so that we can
	// notify the user in an incremental sync.
	err := d.invites.deleteInviteEvent(ctx, inviteEventID)
	return err
}

func (d *SyncServerDatabase) addInvitesToResponse(
	ctx context.Context, txn *sql.Tx,
	userID string,
	fromPos, toPos types.StreamPosition,
	res *types.Response,
) error {
	invites, err := d.invites.selectInviteEventsInRange(
		ctx, txn, userID, int64(fromPos), int64(toPos),
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

// addRoomDeltaToResponse adds a room state delta to a sync response
func (d *SyncServerDatabase) addRoomDeltaToResponse(
	ctx context.Context,
	device *authtypes.Device,
	txn *sql.Tx,
	fromPos, toPos types.StreamPosition,
	delta stateDelta,
	numRecentEventsPerRoom int,
	res *types.Response,
) error {
	endPos := toPos
	if delta.membershipPos > 0 && delta.membership == "leave" {
		// make sure we don't leak recent events after the leave event.
		// TODO: History visibility makes this somewhat complex to handle correctly. For example:
		// TODO: This doesn't work for join -> leave in a single /sync request (see events prior to join).
		// TODO: This will fail on join -> leave -> sensitive msg -> join -> leave
		//       in a single /sync request
		// This is all "okay" assuming history_visibility == "shared" which it is by default.
		endPos = delta.membershipPos
	}
	recentStreamEvents, err := d.events.selectRecentEvents(
		ctx, txn, delta.roomID, fromPos, endPos, numRecentEventsPerRoom,
	)
	if err != nil {
		return err
	}
	recentEvents := streamEventsToEvents(device, recentStreamEvents)
	delta.stateEvents = removeDuplicates(delta.stateEvents, recentEvents) // roll back

	// Don't bother appending empty room entries
	if len(recentEvents) == 0 && len(delta.stateEvents) == 0 {
		return nil
	}

	switch delta.membership {
	case "join":
		jr := types.NewJoinResponse()
		jr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		jr.State.Events = gomatrixserverlib.ToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[delta.roomID] = *jr
	case "leave":
		fallthrough // transitions to leave are the same as ban
	case "ban":
		// TODO: recentEvents may contain events that this user is not allowed to see because they are
		//       no longer in the room.
		lr := types.NewLeaveResponse()
		lr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		lr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		lr.State.Events = gomatrixserverlib.ToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Leave[delta.roomID] = *lr
	}

	return nil
}

// fetchStateEvents converts the set of event IDs into a set of events. It will fetch any which are missing from the database.
// Returns a map of room ID to list of events.
func (d *SyncServerDatabase) fetchStateEvents(
	ctx context.Context, txn *sql.Tx,
	roomIDToEventIDSet map[string]map[string]bool,
	eventIDToEvent map[string]streamEvent,
) (map[string][]streamEvent, error) {
	stateBetween := make(map[string][]streamEvent)
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

func (d *SyncServerDatabase) fetchMissingStateEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]streamEvent, error) {
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

func (d *SyncServerDatabase) getStateDeltas(
	ctx context.Context, device *authtypes.Device, txn *sql.Tx,
	fromPos, toPos types.StreamPosition, userID string,
) ([]stateDelta, error) {
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
	stateNeeded, eventMap, err := d.events.selectStateInRange(ctx, txn, fromPos, toPos)
	if err != nil {
		return nil, err
	}
	state, err := d.fetchStateEvents(ctx, txn, stateNeeded, eventMap)
	if err != nil {
		return nil, err
	}

	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			// TODO: Currently this will incorrectly add rooms which were ALREADY joined but they sent another no-op join event.
			//       We should be checking if the user was already joined at fromPos and not proceed if so. As a result of this,
			//       dupe join events will result in the entire room state coming down to the client again. This is added in
			//       the 'state' part of the response though, so is transparent modulo bandwidth concerns as it is not added to
			//       the timeline.
			if membership := getMembershipFromEvent(&ev.Event, userID); membership != "" {
				if membership == "join" {
					// send full room state down instead of a delta
					var allState []gomatrixserverlib.Event
					allState, err = d.roomstate.selectCurrentState(ctx, txn, roomID)
					if err != nil {
						return nil, err
					}
					s := make([]streamEvent, len(allState))
					for i := 0; i < len(s); i++ {
						s[i] = streamEvent{Event: allState[i], streamPosition: types.StreamPosition(0)}
					}
					state[roomID] = s
					continue // we'll add this room in when we do joined rooms
				}

				deltas = append(deltas, stateDelta{
					membership:    membership,
					membershipPos: ev.streamPosition,
					stateEvents:   streamEventsToEvents(device, stateStreamEvents),
					roomID:        roomID,
				})
				break
			}
		}
	}

	// Add in currently joined rooms
	joinedRoomIDs, err := d.roomstate.selectRoomIDsWithMembership(ctx, txn, userID, "join")
	if err != nil {
		return nil, err
	}
	for _, joinedRoomID := range joinedRoomIDs {
		deltas = append(deltas, stateDelta{
			membership:  "join",
			stateEvents: streamEventsToEvents(device, state[joinedRoomID]),
			roomID:      joinedRoomID,
		})
	}

	return deltas, nil
}

// streamEventsToEvents converts streamEvent to Event. If device is non-nil and
// matches the streamevent.transactionID device then the transaction ID gets
// added to the unsigned section of the output event.
func streamEventsToEvents(device *authtypes.Device, in []streamEvent) []gomatrixserverlib.Event {
	out := make([]gomatrixserverlib.Event, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[i].Event
		if device != nil && in[i].transactionID != nil {
			if device.UserID == in[i].Sender() && device.ID == in[i].transactionID.DeviceID {
				err := out[i].SetUnsignedField(
					"transaction_id", in[i].transactionID.TransactionID,
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

/*
send to device messaging implementation
del / maxID / select in range / insert
*/

// DelStdMessage delete message for a given maxID, those below would be deleted
func (d *SyncServerDatabase) DelStdMessage(
	ctx context.Context, targetUID, targetDevice string, maxID int64,
) (err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		err := d.stdMsg.deleteStdEvent(ctx, targetUID, targetDevice, maxID)
		return err
	})
	return
}

// InsertStdMessage insert std message
func (d *SyncServerDatabase) InsertStdMessage(
	ctx context.Context, stdEvent types.StdHolder, transactionID, targetUID, targetDevice string,
) (pos int64, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		curPos, err := d.stdMsg.insertStdEvent(ctx, stdEvent, transactionID, targetUID, targetDevice)
		pos = curPos
		return err
	})
	return
}

// SelectMaxStdID select maximum id in std stream
func (d *SyncServerDatabase) SelectMaxStdID(
	ctx context.Context,
) (maxID int64, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		max, err := d.stdMsg.selectMaxStdID(ctx, txn)
		maxID = max
		return err
	})
	return
}

// SelectRangedStd select a range of std messages
func (d *SyncServerDatabase) SelectRangedStd(
	ctx context.Context,
	targetUserID, targetDeviceID string,
	endPos int64,
) (holder []types.StdHolder, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		list, err := d.stdMsg.selectStdEventsInRange(ctx, txn, targetUserID, targetDeviceID, endPos)
		holder = list
		return err
	})
	return
}

// StdEXT : send to device extension process
func StdEXT(
	ctx context.Context,
	syncDB *SyncServerDatabase,
	respIn types.Response,
	userID, deviceID string,
	since int64,
) (respOut *types.Response) {
	respOut = &respIn
	// when extension works at the very beginning
	err := syncDB.stdMsg.deleteStdEvent(ctx, userID, deviceID, since)
	if err != nil {
		return
	}
	// when err is nil, these before res should be tagged omitted,
	// when next /sync is coming , and err is nil , all those omitted.
	res, err := syncDB.SelectRangedStd(ctx, userID, deviceID, since)
	if err != nil {
		return
	}
	//toDevice := &types.ToDevice{}
	mid := []types.StdEvent{}
	//toDevice.StdEvent = mid
	for _, val := range res {
		ev := types.StdEvent{}
		ev.Sender = val.Sender
		ev.Type = val.EventTyp
		err := json.Unmarshal(val.Event, &ev.Content)
		if err != nil {
			return
		}
		mid = append(mid, ev)
	}
	respOut.ToDevice.StdEvent = mid
	return
}
