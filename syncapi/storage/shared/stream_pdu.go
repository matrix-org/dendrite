package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type PDUStreamProvider struct {
	StreamProvider
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

func (p *PDUStreamProvider) Setup() {
	p.StreamProvider.Setup()

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := p.DB.OutputEvents.SelectMaxEventID(context.Background(), nil)
	if err != nil {
		return
	}
	p.latest = types.StreamPosition(id)
}

func (p *PDUStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	to := p.LatestPosition(ctx)

	// This needs to be all done in a transaction as we need to do multiple SELECTs, and we need to have
	// a consistent view of the database throughout. This does have the unfortunate side-effect that all
	// the matrixy logic resides in this function, but it's better to not hide the fact that this is
	// being done in a transaction.
	txn, err := p.DB.DB.BeginTx(ctx, &txReadOnlySnapshot)
	if err != nil {
		return to
	}
	succeeded := false
	defer sqlutil.EndTransactionWithCheck(txn, &succeeded, &err)

	// Get the current sync position which we will base the sync response on.
	r := types.Range{
		From: 0,
		To:   to,
	}

	// Extract room state and recent events for all rooms the user is joined to.
	var joinedRoomIDs []string
	joinedRoomIDs, err = p.DB.CurrentRoomState.SelectRoomIDsWithMembership(ctx, txn, req.Device.UserID, gomatrixserverlib.Join)
	if err != nil {
		return to
	}

	stateFilter := gomatrixserverlib.DefaultStateFilter() // TODO: use filter provided in request

	// Build up a /sync response. Add joined rooms.
	for _, roomID := range joinedRoomIDs {
		var jr *types.JoinResponse
		jr, err = p.getJoinResponseForCompleteSync(
			ctx, txn, roomID, r, &stateFilter, 20, req.Device,
		)
		if err != nil {
			return to
		}
		req.Response.Rooms.Join[roomID] = *jr
	}

	// Add peeked rooms.
	peeks, err := p.DB.Peeks.SelectPeeksInRange(ctx, txn, req.Device.UserID, req.Device.ID, r)
	if err != nil {
		return to
	}
	for _, peek := range peeks {
		if !peek.Deleted {
			var jr *types.JoinResponse
			jr, err = p.getJoinResponseForCompleteSync(
				ctx, txn, peek.RoomID, r, &stateFilter, 20, req.Device,
			)
			if err != nil {
				return to
			}
			req.Response.Rooms.Peek[peek.RoomID] = *jr
		}
	}

	succeeded = true

	return p.LatestPosition(ctx)
}

// nolint:gocyclo
func (p *PDUStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) (newPos types.StreamPosition) {
	r := types.Range{
		From:      from,
		To:        to,
		Backwards: from > to,
	}
	newPos = to

	var err error
	var events []types.StreamEvent
	var stateDeltas []stateDelta
	var joinedRooms []string

	// TODO: use filter provided in request
	stateFilter := gomatrixserverlib.DefaultStateFilter()

	if req.WantFullState {
		if stateDeltas, joinedRooms, err = p.DB.getStateDeltasForFullStateSync(ctx, req.Device, nil, r, req.Device.UserID, &stateFilter); err != nil {
			return
		}
	} else {
		if stateDeltas, joinedRooms, err = p.DB.getStateDeltas(ctx, req.Device, nil, r, req.Device.UserID, &stateFilter); err != nil {
			return
		}
	}

	for _, roomID := range joinedRooms {
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	for _, stateDelta := range stateDeltas {
		roomID := stateDelta.roomID
		room := types.JoinResponse{}

		if r.Backwards {
			// When using backward ordering, we want the most recent events first.
			if events, _, err = p.DB.OutputEvents.SelectRecentEvents(ctx, nil, roomID, r, req.Filter.Limit, false, false); err != nil {
				return
			}
		} else {
			// When using forward ordering, we want the least recent events first.
			if events, err = p.DB.OutputEvents.SelectEarlyEvents(ctx, nil, roomID, r, req.Filter.Limit); err != nil {
				return
			}
		}

		for _, event := range p.DB.StreamEventsToEvents(req.Device, events) {
			room.Timeline.Events = append(
				room.Timeline.Events,
				gomatrixserverlib.ToClientEvent(
					event.Event,
					gomatrixserverlib.FormatSync,
				),
			)
		}

		for _, event := range events {
			if event.StreamPosition > newPos {
				newPos = event.StreamPosition
			}
		}

		room.State.Events = gomatrixserverlib.HeaderedToClientEvents(
			stateDelta.stateEvents,
			gomatrixserverlib.FormatSync,
		)

		if len(events) > 0 {
			prevBatch, err := p.DB.getBackwardTopologyPos(ctx, nil, events)
			if err != nil {
				return
			}
			room.Timeline.PrevBatch = &prevBatch
		}

		req.Response.Rooms.Join[roomID] = room
	}

	return newPos
}

func (p *PDUStreamProvider) getJoinResponseForCompleteSync(
	ctx context.Context, txn *sql.Tx,
	roomID string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	numRecentEventsPerRoom int, device *userapi.Device,
) (jr *types.JoinResponse, err error) {
	var stateEvents []*gomatrixserverlib.HeaderedEvent
	stateEvents, err = p.DB.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, stateFilter)
	if err != nil {
		return
	}
	// TODO: When filters are added, we may need to call this multiple times to get enough events.
	//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
	var recentStreamEvents []types.StreamEvent
	var limited bool
	recentStreamEvents, limited, err = p.DB.OutputEvents.SelectRecentEvents(
		ctx, txn, roomID, r, numRecentEventsPerRoom, true, true,
	)
	if err != nil {
		return
	}

	// TODO FIXME: We don't fully implement history visibility yet. To avoid leaking events which the
	// user shouldn't see, we check the recent events and remove any prior to the join event of the user
	// which is equiv to history_visibility: joined
	joinEventIndex := -1
	for i := len(recentStreamEvents) - 1; i >= 0; i-- {
		ev := recentStreamEvents[i]
		if ev.Type() == gomatrixserverlib.MRoomMember && ev.StateKeyEquals(device.UserID) {
			membership, _ := ev.Membership()
			if membership == "join" {
				joinEventIndex = i
				if i > 0 {
					// the create event happens before the first join, so we should cut it at that point instead
					if recentStreamEvents[i-1].Type() == gomatrixserverlib.MRoomCreate && recentStreamEvents[i-1].StateKeyEquals("") {
						joinEventIndex = i - 1
						break
					}
				}
				break
			}
		}
	}
	if joinEventIndex != -1 {
		// cut all events earlier than the join (but not the join itself)
		recentStreamEvents = recentStreamEvents[joinEventIndex:]
		limited = false // so clients know not to try to backpaginate
	}

	// Retrieve the backward topology position, i.e. the position of the
	// oldest event in the room's topology.
	var prevBatch *types.TopologyToken
	if len(recentStreamEvents) > 0 {
		var backwardTopologyPos, backwardStreamPos types.StreamPosition
		backwardTopologyPos, backwardStreamPos, err = p.DB.Topology.SelectPositionInTopology(ctx, txn, recentStreamEvents[0].EventID())
		if err != nil {
			return
		}
		prevBatch = &types.TopologyToken{
			Depth:       backwardTopologyPos,
			PDUPosition: backwardStreamPos,
		}
		prevBatch.Decrement()
	}

	// We don't include a device here as we don't need to send down
	// transaction IDs for complete syncs, but we do it anyway because Sytest demands it for:
	// "Can sync a room with a message with a transaction id" - which does a complete sync to check.
	recentEvents := p.DB.StreamEventsToEvents(device, recentStreamEvents)
	stateEvents = removeDuplicates(stateEvents, recentEvents)
	jr = types.NewJoinResponse()
	jr.Timeline.PrevBatch = prevBatch
	jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
	jr.Timeline.Limited = limited
	jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
	return jr, nil
}

func removeDuplicates(stateEvents, recentEvents []*gomatrixserverlib.HeaderedEvent) []*gomatrixserverlib.HeaderedEvent {
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
