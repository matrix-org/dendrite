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
	"github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"go.uber.org/atomic"

	"github.com/matrix-org/dendrite/syncapi/notifier"
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
	notifier      *notifier.Notifier
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

	// Invalidate the lazyLoadCache, otherwise we end up with missing displaynames/avatars
	// TODO: This might be inefficient, when joined to many and/or large rooms.
	for _, roomID := range joinedRoomIDs {
		joinedUsers := p.notifier.JoinedUsers(roomID)
		for _, sharedUser := range joinedUsers {
			p.lazyLoadCache.InvalidateLazyLoadedUser(req.Device, roomID, sharedUser)
		}
	}

	// Build up a /sync response. Add joined rooms.
	var reqMutex sync.Mutex
	var reqWaitGroup sync.WaitGroup
	reqWaitGroup.Add(len(joinedRoomIDs))
	for _, room := range joinedRoomIDs {
		roomID := room
		p.queue(func() {
			defer reqWaitGroup.Done()

			jr, jerr := p.getJoinResponseForCompleteSync(
				ctx, roomID, r, &stateFilter, &eventFilter, req.WantFullState, req.Device, false,
			)
			if jerr != nil {
				req.Log.WithError(jerr).Error("p.getJoinResponseForCompleteSync failed")
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
				ctx, peek.RoomID, r, &stateFilter, &eventFilter, req.WantFullState, req.Device, true,
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
	var syncJoinedRooms []string
	var newlyJoinedRooms map[string]struct{}

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if req.WantFullState {
		if stateDeltas, syncJoinedRooms, err = p.DB.GetStateDeltasForFullStateSync(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltasForFullStateSync failed")
			return
		}
	} else {
		if stateDeltas, syncJoinedRooms, newlyJoinedRooms, err = p.DB.GetStateDeltas(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltas failed")
			return
		}
	}

	for _, roomID := range syncJoinedRooms {
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
		newRange := r
		// If this room was joined in this sync, try to fetch
		// as much timeline events as allowed by the filter.
		if _, ok := newlyJoinedRooms[delta.RoomID]; ok {
			// Reverse the range, so we get the most recent first.
			// This will be limited by the eventFilter.
			newRange = types.Range{
				From:      r.To,
				To:        0,
				Backwards: true,
			}
		}
		var pos types.StreamPosition
		if pos, err = p.addRoomDeltaToResponse(ctx, req.Device, newRange, delta, &eventFilter, &stateFilter, req.Response); err != nil {
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
			case r.Backwards && pos < latestPosition:
				fallthrough
			case !r.Backwards && pos > latestPosition:
				latestPosition = pos
			}
		}
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

	// Applies the history visibility rules
	events, err := applyHistoryVisibilityFilter(ctx, p.DB, p.rsAPI, delta.RoomID, device.UserID, eventFilter.Limit, recentEvents)
	if err != nil {
		logrus.WithError(err).Error("unable to apply history visibility filter")
	}

	if len(delta.StateEvents) > 0 {
		updateLatestPosition(delta.StateEvents[len(delta.StateEvents)-1].EventID())
	}
	if len(events) > 0 {
		updateLatestPosition(events[len(events)-1].EventID())
	}

	switch delta.Membership {
	case gomatrixserverlib.Join:
		jr := types.NewJoinResponse()
		if hasMembershipChange {
			p.addRoomSummary(ctx, jr, delta.RoomID, device.UserID, latestPosition)
		}
		jr.Timeline.PrevBatch = &prevBatch
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(events, gomatrixserverlib.FormatSync)
		// If we are limited by the filter AND the history visibility filter
		// didn't "remove" events, return that the response is limited.
		jr.Timeline.Limited = limited && len(events) == len(recentEvents)
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[delta.RoomID] = *jr

	case gomatrixserverlib.Peek:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = &prevBatch
		// TODO: Apply history visibility on peeked rooms
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = limited
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Peek[delta.RoomID] = *jr

	case gomatrixserverlib.Leave:
		fallthrough // transitions to leave are the same as ban

	case gomatrixserverlib.Ban:
		lr := types.NewLeaveResponse()
		lr.Timeline.PrevBatch = &prevBatch
		lr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(events, gomatrixserverlib.FormatSync)
		// If we are limited by the filter AND the history visibility filter
		// didn't "remove" events, return that the response is limited.
		lr.Timeline.Limited = limited && len(events) == len(recentEvents)
		lr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Leave[delta.RoomID] = *lr
	}

	return latestPosition, nil
}

// applyHistoryVisibilityFilter gets the current room state and supplies it to ApplyHistoryVisibilityFilter, to make
// sure we always return the required events in the timeline.
func applyHistoryVisibilityFilter(
	ctx context.Context,
	db storage.Database,
	rsAPI roomserverAPI.SyncRoomserverAPI,
	roomID, userID string,
	limit int,
	recentEvents []*gomatrixserverlib.HeaderedEvent,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	// We need to make sure we always include the latest states events, if they are in the timeline.
	// We grep at least limit * 2 events, to ensure we really get the needed events.
	stateEvents, err := db.CurrentState(ctx, roomID, &gomatrixserverlib.StateFilter{Limit: limit * 2}, nil)
	if err != nil {
		// Not a fatal error, we can continue without the stateEvents,
		// they are only needed if there are state events in the timeline.
		logrus.WithError(err).Warnf("failed to get current room state")
	}
	alwaysIncludeIDs := make(map[string]struct{}, len(stateEvents))
	for _, ev := range stateEvents {
		alwaysIncludeIDs[ev.EventID()] = struct{}{}
	}
	startTime := time.Now()
	events, err := internal.ApplyHistoryVisibilityFilter(ctx, db, rsAPI, recentEvents, alwaysIncludeIDs, userID, "sync")
	if err != nil {

		return nil, err
	}
	logrus.WithFields(logrus.Fields{
		"duration": time.Since(startTime),
		"room_id":  roomID,
	}).Debug("applied history visibility (sync)")
	return events, nil
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
	isPeek bool,
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

	events := recentEvents
	// Only apply history visibility checks if the response is for joined rooms
	if !isPeek {
		events, err = applyHistoryVisibilityFilter(ctx, p.DB, p.rsAPI, roomID, device.UserID, eventFilter.Limit, recentEvents)
		if err != nil {
			logrus.WithError(err).Error("unable to apply history visibility filter")
		}
	}

	// If we are limited by the filter AND the history visibility filter
	// didn't "remove" events, return that the response is limited.
	limited = limited && len(events) == len(recentEvents)

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
	jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(events, gomatrixserverlib.FormatSync)
	// If we are limited by the filter AND the history visibility filter
	// didn't "remove" events, return that the response is limited.
	jr.Timeline.Limited = limited && len(events) == len(recentEvents)
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
			stateKey := *event.StateKey()
			if _, ok := timelineUsers[stateKey]; ok || isGappedIncremental {
				newStateEvents = append(newStateEvents, event)
				if !includeRedundant {
					p.lazyLoadCache.StoreLazyLoadedUser(device, roomID, stateKey, event.EventID())
				}
				delete(timelineUsers, stateKey)
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
	filter := gomatrixserverlib.DefaultStateFilter()
	filter.Senders = &wantUsers
	filter.Types = &[]string{gomatrixserverlib.MRoomMember}
	memberships, err := p.DB.GetStateEventsForRoom(ctx, roomID, &filter)
	if err != nil {
		return stateEvents, err
	}
	// cache the membership events
	for _, membership := range memberships {
		p.lazyLoadCache.StoreLazyLoadedUser(device, roomID, *membership.StateKey(), membership.EventID())
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
