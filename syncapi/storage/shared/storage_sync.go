package shared

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type DatabaseTransaction struct {
	*Database
	ctx context.Context
	txn *sql.Tx
}

func (d *DatabaseTransaction) Commit() error {
	if d.txn == nil {
		return nil
	}
	return d.txn.Commit()
}

func (d *DatabaseTransaction) Rollback() error {
	if d.txn == nil {
		return nil
	}
	return d.txn.Rollback()
}

func (d *DatabaseTransaction) MaxStreamPositionForPDUs(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.OutputEvents.SelectMaxEventID(ctx, d.txn)
	if err != nil {
		return 0, fmt.Errorf("d.OutputEvents.SelectMaxEventID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *DatabaseTransaction) MaxStreamPositionForReceipts(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.Receipts.SelectMaxReceiptID(ctx, d.txn)
	if err != nil {
		return 0, fmt.Errorf("d.Receipts.SelectMaxReceiptID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *DatabaseTransaction) MaxStreamPositionForInvites(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.Invites.SelectMaxInviteID(ctx, d.txn)
	if err != nil {
		return 0, fmt.Errorf("d.Invites.SelectMaxInviteID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *DatabaseTransaction) MaxStreamPositionForSendToDeviceMessages(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.SendToDevice.SelectMaxSendToDeviceMessageID(ctx, d.txn)
	if err != nil {
		return 0, fmt.Errorf("d.SendToDevice.SelectMaxSendToDeviceMessageID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *DatabaseTransaction) MaxStreamPositionForAccountData(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.AccountData.SelectMaxAccountDataID(ctx, d.txn)
	if err != nil {
		return 0, fmt.Errorf("d.Invites.SelectMaxAccountDataID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *DatabaseTransaction) MaxStreamPositionForNotificationData(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.NotificationData.SelectMaxID(ctx, d.txn)
	if err != nil {
		return 0, fmt.Errorf("d.NotificationData.SelectMaxID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *DatabaseTransaction) CurrentState(ctx context.Context, roomID string, stateFilterPart *gomatrixserverlib.StateFilter, excludeEventIDs []string) ([]*gomatrixserverlib.HeaderedEvent, error) {
	return d.CurrentRoomState.SelectCurrentState(ctx, d.txn, roomID, stateFilterPart, excludeEventIDs)
}

func (d *DatabaseTransaction) RoomIDsWithMembership(ctx context.Context, userID string, membership string) ([]string, error) {
	return d.CurrentRoomState.SelectRoomIDsWithMembership(ctx, d.txn, userID, membership)
}

func (d *DatabaseTransaction) MembershipCount(ctx context.Context, roomID, membership string, pos types.StreamPosition) (int, error) {
	return d.Memberships.SelectMembershipCount(ctx, d.txn, roomID, membership, pos)
}

func (d *DatabaseTransaction) GetRoomHeroes(ctx context.Context, roomID, userID string, memberships []string) ([]string, error) {
	return d.Memberships.SelectHeroes(ctx, d.txn, roomID, userID, memberships)
}

func (d *DatabaseTransaction) RecentEvents(ctx context.Context, roomID string, r types.Range, eventFilter *gomatrixserverlib.RoomEventFilter, chronologicalOrder bool, onlySyncEvents bool) ([]types.StreamEvent, bool, error) {
	return d.OutputEvents.SelectRecentEvents(ctx, d.txn, roomID, r, eventFilter, chronologicalOrder, onlySyncEvents)
}

func (d *DatabaseTransaction) PositionInTopology(ctx context.Context, eventID string) (pos types.StreamPosition, spos types.StreamPosition, err error) {
	return d.Topology.SelectPositionInTopology(ctx, d.txn, eventID)
}

func (d *DatabaseTransaction) InviteEventsInRange(ctx context.Context, targetUserID string, r types.Range) (map[string]*gomatrixserverlib.HeaderedEvent, map[string]*gomatrixserverlib.HeaderedEvent, types.StreamPosition, error) {
	return d.Invites.SelectInviteEventsInRange(ctx, d.txn, targetUserID, r)
}

func (d *DatabaseTransaction) PeeksInRange(ctx context.Context, userID, deviceID string, r types.Range) (peeks []types.Peek, err error) {
	return d.Peeks.SelectPeeksInRange(ctx, d.txn, userID, deviceID, r)
}

func (d *DatabaseTransaction) RoomReceiptsAfter(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) (types.StreamPosition, []types.OutputReceiptEvent, error) {
	return d.Receipts.SelectRoomReceiptsAfter(ctx, d.txn, roomIDs, streamPos)
}

// Events lookups a list of event by their event ID.
// Returns a list of events matching the requested IDs found in the database.
// If an event is not found in the database then it will be omitted from the list.
// Returns an error if there was a problem talking with the database.
// Does not include any transaction IDs in the returned events.
func (d *DatabaseTransaction) Events(ctx context.Context, eventIDs []string) ([]*gomatrixserverlib.HeaderedEvent, error) {
	streamEvents, err := d.OutputEvents.SelectEvents(ctx, d.txn, eventIDs, nil, false)
	if err != nil {
		return nil, err
	}

	// We don't include a device here as we only include transaction IDs in
	// incremental syncs.
	return d.StreamEventsToEvents(nil, streamEvents), nil
}

func (d *DatabaseTransaction) AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error) {
	return d.CurrentRoomState.SelectJoinedUsers(ctx, d.txn)
}

func (d *DatabaseTransaction) AllJoinedUsersInRoom(ctx context.Context, roomIDs []string) (map[string][]string, error) {
	return d.CurrentRoomState.SelectJoinedUsersInRoom(ctx, d.txn, roomIDs)
}

func (d *DatabaseTransaction) AllPeekingDevicesInRooms(ctx context.Context) (map[string][]types.PeekingDevice, error) {
	return d.Peeks.SelectPeekingDevices(ctx, d.txn)
}

func (d *DatabaseTransaction) SharedUsers(ctx context.Context, userID string, otherUserIDs []string) ([]string, error) {
	return d.CurrentRoomState.SelectSharedUsers(ctx, d.txn, userID, otherUserIDs)
}

func (d *DatabaseTransaction) GetStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.HeaderedEvent, error) {
	return d.CurrentRoomState.SelectStateEvent(ctx, d.txn, roomID, evType, stateKey)
}

func (d *DatabaseTransaction) GetStateEventsForRoom(
	ctx context.Context, roomID string, stateFilter *gomatrixserverlib.StateFilter,
) (stateEvents []*gomatrixserverlib.HeaderedEvent, err error) {
	stateEvents, err = d.CurrentRoomState.SelectCurrentState(ctx, d.txn, roomID, stateFilter, nil)
	return
}

// GetAccountDataInRange returns all account data for a given user inserted or
// updated between two given positions
// Returns a map following the format data[roomID] = []dataTypes
// If no data is retrieved, returns an empty map
// If there was an issue with the retrieval, returns an error
func (d *DatabaseTransaction) GetAccountDataInRange(
	ctx context.Context, userID string, r types.Range,
	accountDataFilterPart *gomatrixserverlib.EventFilter,
) (map[string][]string, types.StreamPosition, error) {
	return d.AccountData.SelectAccountDataInRange(ctx, d.txn, userID, r, accountDataFilterPart)
}

func (d *DatabaseTransaction) GetEventsInTopologicalRange(
	ctx context.Context,
	from, to *types.TopologyToken,
	roomID string,
	filter *gomatrixserverlib.RoomEventFilter,
	backwardOrdering bool,
) (events []types.StreamEvent, err error) {
	var minDepth, maxDepth, maxStreamPosForMaxDepth types.StreamPosition
	if backwardOrdering {
		// Backward ordering means the 'from' token has a higher depth than the 'to' token
		minDepth = to.Depth
		maxDepth = from.Depth
		// for cases where we have say 5 events with the same depth, the TopologyToken needs to
		// know which of the 5 the client has seen. This is done by using the PDU position.
		// Events with the same maxDepth but less than this PDU position will be returned.
		maxStreamPosForMaxDepth = from.PDUPosition
	} else {
		// Forward ordering means the 'from' token has a lower depth than the 'to' token.
		minDepth = from.Depth
		maxDepth = to.Depth
	}

	// Select the event IDs from the defined range.
	var eIDs []string
	eIDs, err = d.Topology.SelectEventIDsInRange(
		ctx, d.txn, roomID, minDepth, maxDepth, maxStreamPosForMaxDepth, filter.Limit, !backwardOrdering,
	)
	if err != nil {
		return
	}

	// Retrieve the events' contents using their IDs.
	events, err = d.OutputEvents.SelectEvents(ctx, d.txn, eIDs, filter, true)
	return
}

func (d *DatabaseTransaction) BackwardExtremitiesForRoom(
	ctx context.Context, roomID string,
) (backwardExtremities map[string][]string, err error) {
	return d.BackwardExtremities.SelectBackwardExtremitiesForRoom(ctx, d.txn, roomID)
}

func (d *DatabaseTransaction) MaxTopologicalPosition(
	ctx context.Context, roomID string,
) (types.TopologyToken, error) {
	depth, streamPos, err := d.Topology.SelectMaxPositionInTopology(ctx, d.txn, roomID)
	if err != nil {
		return types.TopologyToken{}, err
	}
	return types.TopologyToken{Depth: depth, PDUPosition: streamPos}, nil
}

func (d *DatabaseTransaction) EventPositionInTopology(
	ctx context.Context, eventID string,
) (types.TopologyToken, error) {
	depth, stream, err := d.Topology.SelectPositionInTopology(ctx, d.txn, eventID)
	if err != nil {
		return types.TopologyToken{}, err
	}
	return types.TopologyToken{Depth: depth, PDUPosition: stream}, nil
}

func (d *DatabaseTransaction) StreamToTopologicalPosition(
	ctx context.Context, roomID string, streamPos types.StreamPosition, backwardOrdering bool,
) (types.TopologyToken, error) {
	topoPos, err := d.Topology.SelectStreamToTopologicalPosition(ctx, d.txn, roomID, streamPos, backwardOrdering)
	switch {
	case err == sql.ErrNoRows && backwardOrdering: // no events in range, going backward
		return types.TopologyToken{PDUPosition: streamPos}, nil
	case err == sql.ErrNoRows && !backwardOrdering: // no events in range, going forward
		topoPos, streamPos, err = d.Topology.SelectMaxPositionInTopology(ctx, d.txn, roomID)
		if err != nil {
			return types.TopologyToken{}, fmt.Errorf("d.Topology.SelectMaxPositionInTopology: %w", err)
		}
		return types.TopologyToken{Depth: topoPos, PDUPosition: streamPos}, nil
	case err != nil: // some other error happened
		return types.TopologyToken{}, fmt.Errorf("d.Topology.SelectStreamToTopologicalPosition: %w", err)
	default:
		return types.TopologyToken{Depth: topoPos, PDUPosition: streamPos}, nil
	}
}

// GetBackwardTopologyPos retrieves the backward topology position, i.e. the position of the
// oldest event in the room's topology.
func (d *DatabaseTransaction) GetBackwardTopologyPos(
	ctx context.Context,
	events []*gomatrixserverlib.HeaderedEvent,
) (types.TopologyToken, error) {
	zeroToken := types.TopologyToken{}
	if len(events) == 0 {
		return zeroToken, nil
	}
	pos, spos, err := d.Topology.SelectPositionInTopology(ctx, d.txn, events[0].EventID())
	if err != nil {
		return zeroToken, err
	}
	tok := types.TopologyToken{Depth: pos, PDUPosition: spos}
	tok.Decrement()
	return tok, nil
}

// GetStateDeltas returns the state deltas between fromPos and toPos,
// exclusive of oldPos, inclusive of newPos, for the rooms in which
// the user has new membership events.
// A list of joined room IDs is also returned in case the caller needs it.
// nolint:gocyclo
func (d *DatabaseTransaction) GetStateDeltas(
	ctx context.Context, device *userapi.Device,
	r types.Range, userID string,
	stateFilter *gomatrixserverlib.StateFilter,
) (deltas []types.StateDelta, joinedRoomsIDs []string, err error) {
	// Implement membership change algorithm: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L821
	// - Get membership list changes for this user in this sync response
	// - For each room which has membership list changes:
	//     * Check if the room is 'newly joined' (insufficient to just check for a join event because we allow dupe joins TODO).
	//       If it is, then we need to send the full room state down (and 'limited' is always true).
	//     * Check if user is still CURRENTLY invited to the room. If so, add room to 'invited' block.
	//     * Check if the user is CURRENTLY (TODO) left/banned. If so, add room to 'archived' block.
	// - Get all CURRENTLY joined rooms, and add them to 'joined' block.

	// Look up all memberships for the user. We only care about rooms that a
	// user has ever interacted with — joined to, kicked/banned from, left.
	memberships, err := d.CurrentRoomState.SelectRoomIDsWithAnyMembership(ctx, d.txn, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	allRoomIDs := make([]string, 0, len(memberships))
	joinedRoomIDs := make([]string, 0, len(memberships))
	for roomID, membership := range memberships {
		allRoomIDs = append(allRoomIDs, roomID)
		if membership == gomatrixserverlib.Join {
			joinedRoomIDs = append(joinedRoomIDs, roomID)
		}
	}

	// get all the state events ever (i.e. for all available rooms) between these two positions
	stateNeeded, eventMap, err := d.OutputEvents.SelectStateInRange(ctx, d.txn, r, nil, allRoomIDs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	state, err := d.fetchStateEvents(ctx, d.txn, stateNeeded, eventMap)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// get all the state events ever (i.e. for all available rooms) between these two positions
	stateNeededFiltered, eventMapFiltered, err := d.OutputEvents.SelectStateInRange(ctx, d.txn, r, stateFilter, allRoomIDs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	stateFiltered, err := d.fetchStateEvents(ctx, d.txn, stateNeededFiltered, eventMapFiltered)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// find out which rooms this user is peeking, if any.
	// We do this before joins so any peeks get overwritten
	peeks, err := d.Peeks.SelectPeeksInRange(ctx, d.txn, userID, device.ID, r)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, err
	}

	// add peek blocks
	for _, peek := range peeks {
		if peek.New {
			// send full room state down instead of a delta
			var s []types.StreamEvent
			s, err = d.currentStateStreamEventsForRoom(ctx, peek.RoomID, stateFilter)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				}
				return nil, nil, err
			}
			state[peek.RoomID] = s
		}
		if !peek.Deleted {
			deltas = append(deltas, types.StateDelta{
				Membership:  gomatrixserverlib.Peek,
				StateEvents: d.StreamEventsToEvents(device, state[peek.RoomID]),
				RoomID:      peek.RoomID,
			})
		}
	}

	// handle newly joined rooms and non-joined rooms
	newlyJoinedRooms := make(map[string]bool, len(state))
	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			// Look for our membership in the state events and skip over any
			// membership events that are not related to us.
			membership, prevMembership := getMembershipFromEvent(ev.Event, userID)
			if membership == "" {
				continue
			}

			if membership == gomatrixserverlib.Join {
				// If our membership is now join but the previous membership wasn't
				// then this is a "join transition", so we'll insert this room.
				if prevMembership != membership {
					newlyJoinedRooms[roomID] = true
					// Get the full room state, as we'll send that down for a newly
					// joined room instead of a delta.
					var s []types.StreamEvent
					if s, err = d.currentStateStreamEventsForRoom(ctx, roomID, stateFilter); err != nil {
						if err == sql.ErrNoRows {
							continue
						}
						return nil, nil, err
					}

					// Add the information for this room into the state so that
					// it will get added with all of the rest of the joined rooms.
					stateFiltered[roomID] = s
				}

				// We won't add joined rooms into the delta at this point as they
				// are added later on.
				continue
			}

			deltas = append(deltas, types.StateDelta{
				Membership:    membership,
				MembershipPos: ev.StreamPosition,
				StateEvents:   d.StreamEventsToEvents(device, stateFiltered[roomID]),
				RoomID:        roomID,
			})
			break
		}
	}

	// Finally, add in currently joined rooms, including those from the
	// join transitions above.
	for _, joinedRoomID := range joinedRoomIDs {
		deltas = append(deltas, types.StateDelta{
			Membership:  gomatrixserverlib.Join,
			StateEvents: d.StreamEventsToEvents(device, stateFiltered[joinedRoomID]),
			RoomID:      joinedRoomID,
			NewlyJoined: newlyJoinedRooms[joinedRoomID],
		})
	}

	return deltas, joinedRoomIDs, nil
}

// GetStateDeltasForFullStateSync is a variant of getStateDeltas used for /sync
// requests with full_state=true.
// Fetches full state for all joined rooms and uses selectStateInRange to get
// updates for other rooms.
func (d *DatabaseTransaction) GetStateDeltasForFullStateSync(
	ctx context.Context, device *userapi.Device,
	r types.Range, userID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]types.StateDelta, []string, error) {
	// Look up all memberships for the user. We only care about rooms that a
	// user has ever interacted with — joined to, kicked/banned from, left.
	memberships, err := d.CurrentRoomState.SelectRoomIDsWithAnyMembership(ctx, d.txn, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	allRoomIDs := make([]string, 0, len(memberships))
	joinedRoomIDs := make([]string, 0, len(memberships))
	for roomID, membership := range memberships {
		allRoomIDs = append(allRoomIDs, roomID)
		if membership == gomatrixserverlib.Join {
			joinedRoomIDs = append(joinedRoomIDs, roomID)
		}
	}

	// Use a reasonable initial capacity
	deltas := make(map[string]types.StateDelta)

	peeks, err := d.Peeks.SelectPeeksInRange(ctx, d.txn, userID, device.ID, r)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, err
	}

	// Add full states for all peeking rooms
	for _, peek := range peeks {
		if !peek.Deleted {
			s, stateErr := d.currentStateStreamEventsForRoom(ctx, peek.RoomID, stateFilter)
			if stateErr != nil {
				if stateErr == sql.ErrNoRows {
					continue
				}
				return nil, nil, stateErr
			}
			deltas[peek.RoomID] = types.StateDelta{
				Membership:  gomatrixserverlib.Peek,
				StateEvents: d.StreamEventsToEvents(device, s),
				RoomID:      peek.RoomID,
			}
		}
	}

	// Get all the state events ever between these two positions
	stateNeeded, eventMap, err := d.OutputEvents.SelectStateInRange(ctx, d.txn, r, stateFilter, allRoomIDs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	state, err := d.fetchStateEvents(ctx, d.txn, stateNeeded, eventMap)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			if membership, _ := getMembershipFromEvent(ev.Event, userID); membership != "" {
				if membership != gomatrixserverlib.Join { // We've already added full state for all joined rooms above.
					deltas[roomID] = types.StateDelta{
						Membership:    membership,
						MembershipPos: ev.StreamPosition,
						StateEvents:   d.StreamEventsToEvents(device, stateStreamEvents),
						RoomID:        roomID,
					}
				}

				break
			}
		}
	}

	// Add full states for all joined rooms
	for _, joinedRoomID := range joinedRoomIDs {
		s, stateErr := d.currentStateStreamEventsForRoom(ctx, joinedRoomID, stateFilter)
		if stateErr != nil {
			if stateErr == sql.ErrNoRows {
				continue
			}
			return nil, nil, stateErr
		}
		deltas[joinedRoomID] = types.StateDelta{
			Membership:  gomatrixserverlib.Join,
			StateEvents: d.StreamEventsToEvents(device, s),
			RoomID:      joinedRoomID,
		}
	}

	// Create a response array.
	result := make([]types.StateDelta, len(deltas))
	i := 0
	for _, delta := range deltas {
		result[i] = delta
		i++
	}

	return result, joinedRoomIDs, nil
}

func (d *DatabaseTransaction) currentStateStreamEventsForRoom(
	ctx context.Context, roomID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]types.StreamEvent, error) {
	allState, err := d.CurrentRoomState.SelectCurrentState(ctx, d.txn, roomID, stateFilter, nil)
	if err != nil {
		return nil, err
	}
	s := make([]types.StreamEvent, len(allState))
	for i := 0; i < len(s); i++ {
		s[i] = types.StreamEvent{HeaderedEvent: allState[i], StreamPosition: 0}
	}
	return s, nil
}

func (d *DatabaseTransaction) SendToDeviceUpdatesForSync(
	ctx context.Context,
	userID, deviceID string,
	from, to types.StreamPosition,
) (types.StreamPosition, []types.SendToDeviceEvent, error) {
	// First of all, get our send-to-device updates for this user.
	lastPos, events, err := d.SendToDevice.SelectSendToDeviceMessages(ctx, d.txn, userID, deviceID, from, to)
	if err != nil {
		return from, nil, fmt.Errorf("d.SendToDevice.SelectSendToDeviceMessages: %w", err)
	}
	// If there's nothing to do then stop here.
	if len(events) == 0 {
		return to, nil, nil
	}
	return lastPos, events, nil
}

func (d *DatabaseTransaction) GetRoomReceipts(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) ([]types.OutputReceiptEvent, error) {
	_, receipts, err := d.Receipts.SelectRoomReceiptsAfter(ctx, d.txn, roomIDs, streamPos)
	return receipts, err
}

func (d *DatabaseTransaction) GetUserUnreadNotificationCountsForRooms(ctx context.Context, userID string, rooms map[string]string) (map[string]*eventutil.NotificationData, error) {
	roomIDs := make([]string, 0, len(rooms))
	for roomID, membership := range rooms {
		if membership != gomatrixserverlib.Join {
			continue
		}
		roomIDs = append(roomIDs, roomID)
	}
	return d.NotificationData.SelectUserUnreadCountsForRooms(ctx, d.txn, userID, roomIDs)
}

func (d *DatabaseTransaction) GetPresence(ctx context.Context, userID string) (*types.PresenceInternal, error) {
	return d.Presence.GetPresenceForUser(ctx, d.txn, userID)
}

func (d *DatabaseTransaction) PresenceAfter(ctx context.Context, after types.StreamPosition, filter gomatrixserverlib.EventFilter) (map[string]*types.PresenceInternal, error) {
	return d.Presence.GetPresenceAfter(ctx, d.txn, after, filter)
}

func (d *DatabaseTransaction) MaxStreamPositionForPresence(ctx context.Context) (types.StreamPosition, error) {
	return d.Presence.GetMaxPresenceID(ctx, d.txn)
}

func (d *DatabaseTransaction) MaxStreamPositionForRelations(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.Relations.SelectMaxRelationID(ctx, d.txn)
	return types.StreamPosition(id), err
}

func (d *DatabaseTransaction) RelationsFor(ctx context.Context, roomID, eventID, relType, eventType string, from, to types.StreamPosition, backwards bool, limit int) (
	events []types.StreamEvent, prevBatch, nextBatch string, err error,
) {
	r := types.Range{
		From:      from,
		To:        to,
		Backwards: backwards,
	}

	if r.Backwards && r.From == 0 {
		// If we're working backwards (dir=b) and there's no ?from= specified then
		// we will automatically want to work backwards from the current position,
		// so find out what that is.
		if r.From, err = d.MaxStreamPositionForRelations(ctx); err != nil {
			return nil, "", "", fmt.Errorf("d.MaxStreamPositionForRelations: %w", err)
		}
		// The result normally isn't inclusive of the event *at* the ?from=
		// position, so add 1 here so that we include the most recent relation.
		r.From++
	} else if !r.Backwards && r.To == 0 {
		// If we're working forwards (dir=f) and there's no ?to= specified then
		// we will automatically want to work forwards towards the current position,
		// so find out what that is.
		if r.To, err = d.MaxStreamPositionForRelations(ctx); err != nil {
			return nil, "", "", fmt.Errorf("d.MaxStreamPositionForRelations: %w", err)
		}
	}

	// First look up any relations from the database. We add one to the limit here
	// so that we can tell if we're overflowing, as we will only set the "next_batch"
	// in the response if we are.
	relations, _, err := d.Relations.SelectRelationsInRange(ctx, d.txn, roomID, eventID, relType, eventType, r, limit+1)
	if err != nil {
		return nil, "", "", fmt.Errorf("d.Relations.SelectRelationsInRange: %w", err)
	}

	// If we specified a relation type then just get those results, otherwise collate
	// them from all of the returned relation types.
	entries := []types.RelationEntry{}
	if relType != "" {
		entries = relations[relType]
	} else {
		for _, e := range relations {
			entries = append(entries, e...)
		}
	}

	// If there were no entries returned, there were no relations, so stop at this point.
	if len(entries) == 0 {
		return nil, "", "", nil
	}

	// Otherwise, let's try and work out what sensible prev_batch and next_batch values
	// could be. We've requested an extra event by adding one to the limit already so
	// that we can determine whether or not to provide a "next_batch", so trim off that
	// event off the end if needs be.
	if len(entries) > limit {
		entries = entries[:len(entries)-1]
		nextBatch = fmt.Sprintf("%d", entries[len(entries)-1].Position)
	}
	// TODO: set prevBatch? doesn't seem to affect the tests...

	// Extract all of the event IDs from the relation entries so that we can pull the
	// events out of the database. Then go and fetch the events.
	eventIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		eventIDs = append(eventIDs, entry.EventID)
	}
	events, err = d.OutputEvents.SelectEvents(ctx, d.txn, eventIDs, nil, true)
	if err != nil {
		return nil, "", "", fmt.Errorf("d.OutputEvents.SelectEvents: %w", err)
	}

	return events, prevBatch, nextBatch, nil
}
