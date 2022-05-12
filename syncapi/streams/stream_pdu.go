package streams

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
	"go.uber.org/atomic"
)

// The max number of per-room goroutines to have running.
// Too high and this will consume lots of CPU, too low and complete
// sync responses will take longer to process.
const PDU_STREAM_WORKERS = 256

// The maximum number of tasks that can be queued in total before
// backpressure will build up and the rests will start to block.
const PDU_STREAM_QUEUESIZE = PDU_STREAM_WORKERS * 8

type PDUStreamProvider struct {
	StreamProvider

	tasks   chan func()
	workers atomic.Int32
	// userID+deviceID -> lazy loading cache
	lazyLoadCache caching.LazyLoadCache
	rsAPI         roomserverAPI.SyncRoomserverAPI
}

func (p *PDUStreamProvider) worker() {
	defer p.workers.Dec()
	for {
		select {
		case f := <-p.tasks:
			f()
		case <-time.After(time.Second * 10):
			return
		}
	}
}

func (p *PDUStreamProvider) queue(f func()) {
	if p.workers.Load() < PDU_STREAM_WORKERS {
		p.workers.Inc()
		go p.worker()
	}
	p.tasks <- f
}

func (p *PDUStreamProvider) Setup() {
	p.StreamProvider.Setup()
	p.tasks = make(chan func(), PDU_STREAM_QUEUESIZE)

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

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if err = p.addIgnoredUsersToFilter(ctx, req, &eventFilter); err != nil {
		req.Log.WithError(err).Error("unable to update event filter with ignored users")
	}

	// Build up a /sync response. Add joined rooms.
	var reqMutex sync.Mutex
	var reqWaitGroup sync.WaitGroup
	reqWaitGroup.Add(len(joinedRoomIDs))
	for _, room := range joinedRoomIDs {
		roomID := room
		p.queue(func() {
			defer reqWaitGroup.Done()

			var jr *types.JoinResponse
			jr, err = p.getJoinResponseForCompleteSync(
				ctx, roomID, r, &stateFilter, &eventFilter, req.WantFullState, req.Device,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
				return
			}

			reqMutex.Lock()
			defer reqMutex.Unlock()
			req.Response.Rooms.Join[roomID] = *jr
			req.Rooms[roomID] = gomatrixserverlib.Join
		})
	}

	reqWaitGroup.Wait()

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
				ctx, peek.RoomID, r, &stateFilter, &eventFilter, req.WantFullState, req.Device,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
				return from
			}
			req.Response.Rooms.Peek[peek.RoomID] = *jr
		}
	}

	return to
}

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

	if len(stateDeltas) == 0 {
		return to
	}

	if err = p.addIgnoredUsersToFilter(ctx, req, &eventFilter); err != nil {
		req.Log.WithError(err).Error("unable to update event filter with ignored users")
	}

	newPos = from
	for _, delta := range stateDeltas {
		var pos types.StreamPosition
		if pos, err = p.addRoomDeltaToResponse(ctx, req.Device, r, delta, &eventFilter, &stateFilter, req.Response); err != nil {
			req.Log.WithError(err).Error("d.addRoomDeltaToResponse failed")
			return to
		}
		switch {
		case r.Backwards && pos < newPos:
			fallthrough
		case !r.Backwards && pos > newPos:
			newPos = pos
		}
	}

	return newPos
}

// nolint:gocyclo
func (p *PDUStreamProvider) addRoomDeltaToResponse(
	ctx context.Context,
	device *userapi.Device,
	r types.Range,
	delta types.StateDelta,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	stateFilter *gomatrixserverlib.StateFilter,
	res *types.Response,
) (types.StreamPosition, error) {
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
		if err == sql.ErrNoRows {
			return r.To, nil
		}
		return r.From, fmt.Errorf("p.DB.RecentEvents: %w", err)
	}
	recentEvents := p.DB.StreamEventsToEvents(device, recentStreamEvents)
	delta.StateEvents = removeDuplicates(delta.StateEvents, recentEvents) // roll back
	prevBatch, err := p.DB.GetBackwardTopologyPos(ctx, recentStreamEvents)
	if err != nil {
		return r.From, fmt.Errorf("p.DB.GetBackwardTopologyPos: %w", err)
	}

	// If we didn't return any events at all then don't bother doing anything else.
	if len(recentEvents) == 0 && len(delta.StateEvents) == 0 {
		return r.To, nil
	}

	// Sort the events so that we can pick out the latest events from both sections.
	recentEvents = gomatrixserverlib.HeaderedReverseTopologicalOrdering(recentEvents, gomatrixserverlib.TopologicalOrderByPrevEvents)
	delta.StateEvents = gomatrixserverlib.HeaderedReverseTopologicalOrdering(delta.StateEvents, gomatrixserverlib.TopologicalOrderByAuthEvents)

	// Work out what the highest stream position is for all of the events in this
	// room that were returned.
	latestPosition := r.To
	updateLatestPosition := func(mostRecentEventID string) {
		var pos types.StreamPosition
		if _, pos, err = p.DB.PositionInTopology(ctx, mostRecentEventID); err == nil {
			switch {
			case r.Backwards && pos > latestPosition:
				fallthrough
			case !r.Backwards && pos < latestPosition:
				latestPosition = pos
			}
		}
	}
	if len(recentEvents) > 0 {
		updateLatestPosition(recentEvents[len(recentEvents)-1].EventID())
	}
	if len(delta.StateEvents) > 0 {
		updateLatestPosition(delta.StateEvents[len(delta.StateEvents)-1].EventID())
	}

	if stateFilter.LazyLoadMembers {
		delta.StateEvents, err = p.lazyLoadMembers(
			ctx, delta.RoomID, true, limited, stateFilter.IncludeRedundantMembers,
			device, recentEvents, delta.StateEvents,
		)
		if err != nil && err != sql.ErrNoRows {
			return r.From, fmt.Errorf("p.lazyLoadMembers: %w", err)
		}
	}

	hasMembershipChange := false
	for _, recentEvent := range recentStreamEvents {
		if recentEvent.Type() == gomatrixserverlib.MRoomMember && recentEvent.StateKey() != nil {
			hasMembershipChange = true
			break
		}
	}

	switch delta.Membership {
	case gomatrixserverlib.Join:
		jr := types.NewJoinResponse()
		if hasMembershipChange {
			p.addRoomSummary(ctx, jr, delta.RoomID, device.UserID, latestPosition)
		}
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

	return latestPosition, nil
}

func (p *PDUStreamProvider) addRoomSummary(ctx context.Context, jr *types.JoinResponse, roomID, userID string, latestPosition types.StreamPosition) {
	// Work out how many members are in the room.
	joinedCount, _ := p.DB.MembershipCount(ctx, roomID, gomatrixserverlib.Join, latestPosition)
	invitedCount, _ := p.DB.MembershipCount(ctx, roomID, gomatrixserverlib.Invite, latestPosition)

	jr.Summary.JoinedMemberCount = &joinedCount
	jr.Summary.InvitedMemberCount = &invitedCount

	fetchStates := []gomatrixserverlib.StateKeyTuple{
		{EventType: gomatrixserverlib.MRoomName},
		{EventType: gomatrixserverlib.MRoomCanonicalAlias},
	}
	// Check if the room has a name or a canonical alias
	latestState := &roomserverAPI.QueryLatestEventsAndStateResponse{}
	err := p.rsAPI.QueryLatestEventsAndState(ctx, &roomserverAPI.QueryLatestEventsAndStateRequest{StateToFetch: fetchStates, RoomID: roomID}, latestState)
	if err != nil {
		return
	}
	// Check if the room has a name or canonical alias, if so, return.
	for _, ev := range latestState.StateEvents {
		switch ev.Type() {
		case gomatrixserverlib.MRoomName:
			if gjson.GetBytes(ev.Content(), "name").Str != "" {
				return
			}
		case gomatrixserverlib.MRoomCanonicalAlias:
			if gjson.GetBytes(ev.Content(), "alias").Str != "" {
				return
			}
		}
	}
	heroes, err := p.DB.GetRoomHeroes(ctx, roomID, userID, []string{"join", "invite"})
	if err != nil {
		return
	}
	sort.Strings(heroes)
	jr.Summary.Heroes = heroes
}

func (p *PDUStreamProvider) getJoinResponseForCompleteSync(
	ctx context.Context,
	roomID string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	wantFullState bool,
	device *userapi.Device,
) (jr *types.JoinResponse, err error) {
	jr = types.NewJoinResponse()
	// TODO: When filters are added, we may need to call this multiple times to get enough events.
	//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
	recentStreamEvents, limited, err := p.DB.RecentEvents(
		ctx, roomID, r, eventFilter, true, true,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return jr, nil
		}
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

	// Work our way through the timeline events and pick out the event IDs
	// of any state events that appear in the timeline. We'll specifically
	// exclude them at the next step, so that we don't get duplicate state
	// events in both `recentStreamEvents` and `stateEvents`.
	var excludingEventIDs []string
	if !wantFullState {
		excludingEventIDs = make([]string, 0, len(recentStreamEvents))
		for _, event := range recentStreamEvents {
			if event.StateKey() != nil {
				excludingEventIDs = append(excludingEventIDs, event.EventID())
			}
		}
	}

	stateEvents, err := p.DB.CurrentState(ctx, roomID, stateFilter, excludingEventIDs)
	if err != nil {
		return
	}

	// Retrieve the backward topology position, i.e. the position of the
	// oldest event in the room's topology.
	var prevBatch *types.TopologyToken
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

	p.addRoomSummary(ctx, jr, roomID, device.UserID, r.From)

	// We don't include a device here as we don't need to send down
	// transaction IDs for complete syncs, but we do it anyway because Sytest demands it for:
	// "Can sync a room with a message with a transaction id" - which does a complete sync to check.
	recentEvents := p.DB.StreamEventsToEvents(device, recentStreamEvents)
	stateEvents = removeDuplicates(stateEvents, recentEvents)

	if stateFilter.LazyLoadMembers {
		if err != nil {
			return nil, err
		}
		stateEvents, err = p.lazyLoadMembers(ctx, roomID,
			false, limited, stateFilter.IncludeRedundantMembers,
			device, recentEvents, stateEvents,
		)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
	}

	jr.Timeline.PrevBatch = prevBatch
	jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
	jr.Timeline.Limited = limited
	jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
	return jr, nil
}

func (p *PDUStreamProvider) lazyLoadMembers(
	ctx context.Context, roomID string,
	incremental, limited, includeRedundant bool,
	device *userapi.Device,
	timelineEvents, stateEvents []*gomatrixserverlib.HeaderedEvent,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	if len(timelineEvents) == 0 {
		return stateEvents, nil
	}
	// Work out which memberships to include
	timelineUsers := make(map[string]struct{})
	if !incremental {
		timelineUsers[device.UserID] = struct{}{}
	}
	// Add all users the client doesn't know about yet to a list
	for _, event := range timelineEvents {
		// Membership is not yet cached, add it to the list
		if _, ok := p.lazyLoadCache.IsLazyLoadedUserCached(device, roomID, event.Sender()); !ok {
			timelineUsers[event.Sender()] = struct{}{}
		}
	}
	// Preallocate with the same amount, even if it will end up with fewer values
	newStateEvents := make([]*gomatrixserverlib.HeaderedEvent, 0, len(stateEvents))
	// Remove existing membership events we don't care about, e.g. users not in the timeline.events
	for _, event := range stateEvents {
		if event.Type() == gomatrixserverlib.MRoomMember && event.StateKey() != nil {
			// If this is a gapped incremental sync, we still want this membership
			isGappedIncremental := limited && incremental
			// We want this users membership event, keep it in the list
			_, ok := timelineUsers[event.Sender()]
			wantMembership := ok || isGappedIncremental
			if wantMembership {
				newStateEvents = append(newStateEvents, event)
				if !includeRedundant {
					p.lazyLoadCache.StoreLazyLoadedUser(device, roomID, event.Sender(), event.EventID())
				}
				delete(timelineUsers, event.Sender())
			}
		} else {
			newStateEvents = append(newStateEvents, event)
		}
	}
	wantUsers := make([]string, 0, len(timelineUsers))
	for userID := range timelineUsers {
		wantUsers = append(wantUsers, userID)
	}
	// Query missing membership events
	memberships, err := p.DB.GetStateEventsForRoom(ctx, roomID, &gomatrixserverlib.StateFilter{
		Limit:   100,
		Senders: &wantUsers,
		Types:   &[]string{gomatrixserverlib.MRoomMember},
	})
	if err != nil {
		return stateEvents, err
	}
	// cache the membership events
	for _, membership := range memberships {
		p.lazyLoadCache.StoreLazyLoadedUser(device, roomID, membership.Sender(), membership.EventID())
	}
	stateEvents = append(newStateEvents, memberships...)
	return stateEvents, nil
}

// addIgnoredUsersToFilter adds ignored users to the eventfilter and
// the syncreq itself for further use in streams.
func (p *PDUStreamProvider) addIgnoredUsersToFilter(ctx context.Context, req *types.SyncRequest, eventFilter *gomatrixserverlib.RoomEventFilter) error {
	ignores, err := p.DB.IgnoresForUser(ctx, req.Device.UserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	req.IgnoredUsers = *ignores
	userList := make([]string, 0, len(ignores.List))
	for userID := range ignores.List {
		userList = append(userList, userID)
	}
	if len(userList) > 0 {
		eventFilter.NotSenders = &userList
	}
	return nil
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
