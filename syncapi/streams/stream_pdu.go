package streams

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type PDUStreamProvider struct {
	StreamProvider
}

func (p *PDUStreamProvider) Setup() {
	p.StreamProvider.Setup()

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := p.DB.MaxStreamPositionForPDUs(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *PDUStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	from := types.StreamPosition(0)
	to := p.LatestPosition(ctx)

	// Get the current sync position which we will base the sync response on.
	// For complete syncs, we want to start at the most recent events and work
	// backwards, so that we show the most recent events in the room.
	r := types.Range{
		From:      to,
		To:        0,
		Backwards: true,
	}

	// Extract room state and recent events for all rooms the user is joined to.
	joinedRoomIDs, err := p.DB.RoomIDsWithMembership(ctx, req.Device.UserID, gomatrixserverlib.Join)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.RoomIDsWithMembership failed")
		return from
	}

	// Build up a /sync response. Add joined rooms.
	for _, roomID := range joinedRoomIDs {
		var jr *types.JoinResponse
		jr, err = p.getJoinResponseForCompleteSync(
			ctx, roomID, r, &req.Filter, req.Device,
		)
		if err != nil {
			req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
			return from
		}
		req.Response.Rooms.Join[roomID] = *jr
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	// Add peeked rooms.
	peeks, err := p.DB.PeeksInRange(ctx, req.Device.UserID, req.Device.ID, r)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.PeeksInRange failed")
		return from
	}
	for _, peek := range peeks {
		if !peek.Deleted {
			var jr *types.JoinResponse
			jr, err = p.getJoinResponseForCompleteSync(
				ctx, peek.RoomID, r, &req.Filter, req.Device,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
				return from
			}
			req.Response.Rooms.Peek[peek.RoomID] = *jr
		}
	}

	if req.Filter.Room.IncludeLeave {
		var leaveRoomIDs []string
		// Extract room state and recent events for all rooms the user has left
		leaveRoomIDs, err := p.DB.RoomIDsWithMembership(ctx, req.Device.UserID, gomatrixserverlib.Leave)
		if err != nil {
			req.Log.WithError(err).Error("p.DB.RoomIDsWithMembership failed")
			return from
		}
		// Build up a /sync response. Add leave rooms.
		for _, roomID := range leaveRoomIDs {
			var lr *types.LeaveResponse
			lr, err = p.getLeaveResponseForCompleteSync(
				ctx, roomID, r, &req.Filter, req.Device,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getLeaveResponseForCompleteSync failed")
				return from
			}
			req.Response.Rooms.Leave[roomID] = *lr
		}
	}

	return to
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
	var stateDeltas []types.StateDelta
	var joinedRooms []string

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if req.WantFullState {
		if stateDeltas, joinedRooms, err = p.DB.GetStateDeltasForFullStateSync(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltasForFullStateSync failed")
			return
		}
	} else {
		if stateDeltas, joinedRooms, err = p.DB.GetStateDeltas(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltas failed")
			return
		}
	}

	for _, roomID := range joinedRooms {
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	for _, delta := range stateDeltas {
		if err = p.addRoomDeltaToResponse(ctx, req.Device, r, delta, &eventFilter, req.Response); err != nil {
			req.Log.WithError(err).Error("d.addRoomDeltaToResponse failed")
			return newPos
		}
	}

	return r.To
}

func (p *PDUStreamProvider) addRoomDeltaToResponse(
	ctx context.Context,
	device *userapi.Device,
	r types.Range,
	delta types.StateDelta,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	res *types.Response,
) error {
	if delta.MembershipPos > 0 && delta.Membership == gomatrixserverlib.Leave {
		// make sure we don't leak recent events after the leave event.
		// TODO: History visibility makes this somewhat complex to handle correctly. For example:
		// TODO: This doesn't work for join -> leave in a single /sync request (see events prior to join).
		// TODO: This will fail on join -> leave -> sensitive msg -> join -> leave
		//       in a single /sync request
		// This is all "okay" assuming history_visibility == "shared" which it is by default.
		r.To = delta.MembershipPos
	}
	recentStreamEvents, limited, err := p.DB.RecentEvents(
		ctx, delta.RoomID, r,
		eventFilter, true, true,
	)
	if err != nil {
		return err
	}

	recentEvents := p.DB.StreamEventsToEvents(device, recentStreamEvents)
	delta.StateEvents = removeDuplicates(delta.StateEvents, recentEvents) // roll back
	prevBatch, err := p.DB.GetBackwardTopologyPos(ctx, recentStreamEvents)
	if err != nil {
		return err
	}

	// XXX: should we ever get this far if we have no recent events or state in this room?
	// in practice we do for peeks, but possibly not joins?
	if len(recentEvents) == 0 && len(delta.StateEvents) == 0 {
		return nil
	}

	switch delta.Membership {
	case gomatrixserverlib.Join:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = &prevBatch
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = limited
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[delta.RoomID] = *jr

	case gomatrixserverlib.Peek:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = &prevBatch
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = limited
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Peek[delta.RoomID] = *jr

	case gomatrixserverlib.Leave:
		fallthrough // transitions to leave are the same as ban

	case gomatrixserverlib.Ban:
		// TODO: recentEvents may contain events that this user is not allowed to see because they are
		//       no longer in the room.
		lr := types.NewLeaveResponse()
		lr.Timeline.PrevBatch = &prevBatch
		lr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		lr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		lr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Leave[delta.RoomID] = *lr
	}

	return nil
}

func (p *PDUStreamProvider) getResponseForCompleteSync(
	ctx context.Context,
	roomID string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	device *userapi.Device,
) (
	recentEvents, stateEvents []*gomatrixserverlib.HeaderedEvent,
	prevBatch *types.TopologyToken, limited bool, err error,
) {
	stateEvents, err = p.DB.CurrentState(ctx, roomID, stateFilter)
	if err != nil {
		return
	}
	// TODO: When filters are added, we may need to call this multiple times to get enough events.
	//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
	var recentStreamEvents []types.StreamEvent
	recentStreamEvents, limited, err = p.DB.RecentEvents(
		ctx, roomID, r, eventFilter, true, true,
	)
	if err != nil {
		return
	}

	recentStreamEvents, limited = p.filterStreamEventsAccordingToHistoryVisibility(recentStreamEvents, stateEvents, device, limited)

	for _, event := range recentStreamEvents {
		if event.HeaderedEvent.Event.StateKey() != nil {
			stateEvents = append(stateEvents, event.HeaderedEvent)
		}
	}

	// Retrieve the backward topology position, i.e. the position of the
	// oldest event in the room's topology.
	if len(recentStreamEvents) > 0 {
		var backwardTopologyPos, backwardStreamPos types.StreamPosition
		backwardTopologyPos, backwardStreamPos, err = p.DB.PositionInTopology(ctx, recentStreamEvents[0].EventID())
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
	recentEvents = p.DB.StreamEventsToEvents(device, recentStreamEvents)
	stateEvents = removeDuplicates(stateEvents, recentEvents)
	return
}

func (p *PDUStreamProvider) getJoinResponseForCompleteSync(
	ctx context.Context,
	roomID string,
	r types.Range,
	filter *gomatrixserverlib.Filter,
	device *userapi.Device,
) (jr *types.JoinResponse, err error) {
	recentEvents, stateEvents, prevBatch, limited, err := p.getResponseForCompleteSync(
		ctx, roomID, r, &filter.Room.State, &filter.Room.Timeline, device,
	)
	if err != nil {
		return nil, fmt.Errorf("p.getResponseForCompleteSync: %w", err)
	}

	jr = types.NewJoinResponse()
	jr.Timeline.PrevBatch = prevBatch
	jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
	jr.Timeline.Limited = limited
	jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
	return jr, nil
}

func (p *PDUStreamProvider) getLeaveResponseForCompleteSync(
	ctx context.Context,
	roomID string,
	r types.Range,
	filter *gomatrixserverlib.Filter,
	device *userapi.Device,
) (lr *types.LeaveResponse, err error) {
	recentEvents, stateEvents, prevBatch, limited, err := p.getResponseForCompleteSync(
		ctx, roomID, r, &filter.Room.State, &filter.Room.Timeline, device,
	)
	if err != nil {
		return nil, fmt.Errorf("p.getResponseForCompleteSync: %w", err)
	}

	lr = types.NewLeaveResponse()
	lr.Timeline.PrevBatch = prevBatch
	lr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
	lr.Timeline.Limited = limited
	lr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
	return lr, nil
}

// nolint:gocyclo
func (p *PDUStreamProvider) filterStreamEventsAccordingToHistoryVisibility(
	recentStreamEvents []types.StreamEvent,
	stateEvents []*gomatrixserverlib.HeaderedEvent,
	device *userapi.Device,
	limited bool,
) ([]types.StreamEvent, bool) {
	// If the history is world_readable or shared then don't filter.
	for _, stateEvent := range stateEvents {
		if stateEvent.Type() == gomatrixserverlib.MRoomHistoryVisibility {
			var content struct {
				HistoryVisibility string `json:"history_visibility"`
			}
			if err := json.Unmarshal(stateEvent.Content(), &content); err != nil {
				break
			}
			switch content.HistoryVisibility {
			case "world_readable", "shared":
				return recentStreamEvents, limited
			default:
				break
			}
		}
	}

	// TODO FIXME: We don't fully implement history visibility yet. To avoid leaking events which the
	// user shouldn't see, we check the recent events and remove any prior to the join event of the user
	// which is equiv to history_visibility: joined
	joinEventIndex := -1
	leaveEventIndex := -1
	for i := len(recentStreamEvents) - 1; i >= 0; i-- {
		ev := recentStreamEvents[i]
		if ev.Type() == gomatrixserverlib.MRoomMember && ev.StateKeyEquals(device.UserID) {
			membership, _ := ev.Membership()
			if membership == gomatrixserverlib.Join {
				joinEventIndex = i
				if i > 0 {
					// the create event happens before the first join, so we should cut it at that point instead
					if recentStreamEvents[i-1].Type() == gomatrixserverlib.MRoomCreate && recentStreamEvents[i-1].StateKeyEquals("") {
						joinEventIndex = i - 1
					}
				}
				break
			} else if membership == gomatrixserverlib.Leave {
				leaveEventIndex = i
			}

			if joinEventIndex != -1 && leaveEventIndex != -1 {
				break
			}
		}
	}

	// Default at the start of the array
	sliceStart := 0
	// If there is a joinEvent, then cut all events earlier the join
	if joinEventIndex != -1 {
		sliceStart = joinEventIndex
		limited = false // so clients know not to try to backpaginate
	}
	// Default to spanning the rest of the array
	sliceEnd := len(recentStreamEvents)
	// If there is a leaveEvent, then cut all events after the person left
	if leaveEventIndex != -1 {
		sliceEnd = leaveEventIndex + 1
	}

	return recentStreamEvents[sliceStart:sliceEnd], limited
}

func removeDuplicates(stateEvents, recentEvents []*gomatrixserverlib.HeaderedEvent) []*gomatrixserverlib.HeaderedEvent {
	timeline := map[string]struct{}{}
	for _, event := range recentEvents {
		if event.StateKey() == nil {
			continue
		}
		timeline[event.EventID()] = struct{}{}
	}
	state := []*gomatrixserverlib.HeaderedEvent{}
	for _, event := range stateEvents {
		if _, ok := timeline[event.EventID()]; ok {
			continue
		}
		state = append(state, event)
	}
	return state
}
