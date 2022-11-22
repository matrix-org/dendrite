package streams

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
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
	DefaultStreamProvider

	// userID+deviceID -> lazy loading cache
	lazyLoadCache caching.LazyLoadCache
	rsAPI         roomserverAPI.SyncRoomserverAPI
	notifier      *notifier.Notifier
}

func (p *PDUStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseTransaction,
) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := snapshot.MaxStreamPositionForPDUs(ctx)
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *PDUStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
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
	joinedRoomIDs, err := snapshot.RoomIDsWithMembership(ctx, req.Device.UserID, gomatrixserverlib.Join)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.RoomIDsWithMembership failed")
		return from
	}

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if err = p.addIgnoredUsersToFilter(ctx, snapshot, req, &eventFilter); err != nil {
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
	for _, roomID := range joinedRoomIDs {
		jr, jerr := p.getJoinResponseForCompleteSync(
			ctx, snapshot, roomID, r, &stateFilter, &eventFilter, req.WantFullState, req.Device, false,
		)
		if jerr != nil {
			req.Log.WithError(jerr).Error("p.getJoinResponseForCompleteSync failed")
			if ctxErr := req.Context.Err(); ctxErr != nil || jerr == sql.ErrTxDone {
				return from
			}
			continue
		}
		req.Response.Rooms.Join[roomID] = jr
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	// Add peeked rooms.
	peeks, err := snapshot.PeeksInRange(ctx, req.Device.UserID, req.Device.ID, r)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.PeeksInRange failed")
		return from
	}
	for _, peek := range peeks {
		if !peek.Deleted {
			var jr *types.JoinResponse
			jr, err = p.getJoinResponseForCompleteSync(
				ctx, snapshot, peek.RoomID, r, &stateFilter, &eventFilter, req.WantFullState, req.Device, true,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
				if err == context.DeadlineExceeded || err == context.Canceled || err == sql.ErrTxDone {
					return from
				}
				continue
			}
			req.Response.Rooms.Peek[peek.RoomID] = jr
		}
	}

	return to
}

func (p *PDUStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
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

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if req.WantFullState {
		if stateDeltas, syncJoinedRooms, err = snapshot.GetStateDeltasForFullStateSync(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltasForFullStateSync failed")
			return from
		}
	} else {
		if stateDeltas, syncJoinedRooms, err = snapshot.GetStateDeltas(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltas failed")
			return from
		}
	}

	for _, roomID := range syncJoinedRooms {
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	if len(stateDeltas) == 0 {
		return to
	}

	if err = p.addIgnoredUsersToFilter(ctx, snapshot, req, &eventFilter); err != nil {
		req.Log.WithError(err).Error("unable to update event filter with ignored users")
	}

	newPos = from
	for _, delta := range stateDeltas {
		newRange := r
		// If this room was joined in this sync, try to fetch
		// as much timeline events as allowed by the filter.
		if delta.NewlyJoined {
			// Reverse the range, so we get the most recent first.
			// This will be limited by the eventFilter.
			newRange = types.Range{
				From:      r.To,
				To:        0,
				Backwards: true,
			}
		}
		var pos types.StreamPosition
		if pos, err = p.addRoomDeltaToResponse(ctx, snapshot, req.Device, newRange, delta, &eventFilter, &stateFilter, req); err != nil {
			req.Log.WithError(err).Error("d.addRoomDeltaToResponse failed")
			if err == context.DeadlineExceeded || err == context.Canceled || err == sql.ErrTxDone {
				return newPos
			}
			continue
		}
		// Reset the position, as it is only for the special case of newly joined rooms
		if delta.NewlyJoined {
			pos = newRange.From
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

// Limit the recent events to X when going backwards
const recentEventBackwardsLimit = 100

// nolint:gocyclo
func (p *PDUStreamProvider) addRoomDeltaToResponse(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	device *userapi.Device,
	r types.Range,
	delta types.StateDelta,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	stateFilter *gomatrixserverlib.StateFilter,
	req *types.SyncRequest,
) (types.StreamPosition, error) {

	originalLimit := eventFilter.Limit
	// If we're going backwards, grep at least X events, this is mostly to satisfy Sytest
	if r.Backwards && originalLimit < recentEventBackwardsLimit {
		eventFilter.Limit = recentEventBackwardsLimit // TODO: Figure out a better way
		diff := r.From - r.To
		if diff > 0 && diff < recentEventBackwardsLimit {
			eventFilter.Limit = int(diff)
		}
	}

	recentStreamEvents, limited, err := snapshot.RecentEvents(
		ctx, delta.RoomID, r,
		eventFilter, true, true,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return r.To, nil
		}
		return r.From, fmt.Errorf("p.DB.RecentEvents: %w", err)
	}
	recentEvents := gomatrixserverlib.HeaderedReverseTopologicalOrdering(
		snapshot.StreamEventsToEvents(device, recentStreamEvents),
		gomatrixserverlib.TopologicalOrderByPrevEvents,
	)

	// If we didn't return any events at all then don't bother doing anything else.
	if len(recentEvents) == 0 && len(delta.StateEvents) == 0 {
		return r.To, nil
	}

	// Work out what the highest stream position is for all of the events in this
	// room that were returned.
	latestPosition := r.To
	if r.Backwards {
		latestPosition = r.From
	}
	updateLatestPosition := func(mostRecentEventID string) {
		var pos types.StreamPosition
		if _, pos, err = snapshot.PositionInTopology(ctx, mostRecentEventID); err == nil {
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
			ctx, snapshot, delta.RoomID, true, limited, stateFilter,
			device, recentEvents, delta.StateEvents,
		)
		if err != nil && err != sql.ErrNoRows {
			return r.From, fmt.Errorf("p.lazyLoadMembers: %w", err)
		}
	}

	hasMembershipChange := false
	for _, recentEvent := range recentStreamEvents {
		if recentEvent.Type() == gomatrixserverlib.MRoomMember && recentEvent.StateKey() != nil {
			if membership, _ := recentEvent.Membership(); membership == gomatrixserverlib.Join {
				req.MembershipChanges[*recentEvent.StateKey()] = struct{}{}
			}
			hasMembershipChange = true
		}
	}

	// Applies the history visibility rules
	events, err := applyHistoryVisibilityFilter(ctx, snapshot, p.rsAPI, delta.RoomID, device.UserID, recentEvents)
	if err != nil {
		logrus.WithError(err).Error("unable to apply history visibility filter")
	}

	if r.Backwards && len(events) > originalLimit {
		// We're going backwards and the events are ordered chronologically, so take the last `limit` events
		events = events[len(events)-originalLimit:]
		limited = true
	}

	prevBatch, err := snapshot.GetBackwardTopologyPos(ctx, events)
	if err != nil {
		return r.From, fmt.Errorf("p.DB.GetBackwardTopologyPos: %w", err)
	}

	// Now that we've filtered the timeline, work out which state events are still
	// left. Anything that appears in the filtered timeline will be removed from the
	// "state" section and kept in "timeline".
	delta.StateEvents = gomatrixserverlib.HeaderedReverseTopologicalOrdering(
		removeDuplicates(delta.StateEvents, events),
		gomatrixserverlib.TopologicalOrderByAuthEvents,
	)

	if len(delta.StateEvents) > 0 {
		if last := delta.StateEvents[len(delta.StateEvents)-1]; last != nil {
			updateLatestPosition(last.EventID())
		}
	}
	if len(events) > 0 {
		if last := events[len(events)-1]; last != nil {
			updateLatestPosition(last.EventID())
		}
	}

	switch delta.Membership {
	case gomatrixserverlib.Join:
		jr := types.NewJoinResponse()
		if hasMembershipChange {
			p.addRoomSummary(ctx, snapshot, jr, delta.RoomID, device.UserID, latestPosition)
		}
		jr.Timeline.PrevBatch = &prevBatch
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(events, gomatrixserverlib.FormatSync)
		// If we are limited by the filter AND the history visibility filter
		// didn't "remove" events, return that the response is limited.
		jr.Timeline.Limited = (limited && len(events) == len(recentEvents)) || delta.NewlyJoined
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		req.Response.Rooms.Join[delta.RoomID] = jr

	case gomatrixserverlib.Peek:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = &prevBatch
		// TODO: Apply history visibility on peeked rooms
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = limited
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		req.Response.Rooms.Peek[delta.RoomID] = jr

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
		req.Response.Rooms.Leave[delta.RoomID] = lr
	}

	return latestPosition, nil
}

// applyHistoryVisibilityFilter gets the current room state and supplies it to ApplyHistoryVisibilityFilter, to make
// sure we always return the required events in the timeline.
func applyHistoryVisibilityFilter(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	rsAPI roomserverAPI.SyncRoomserverAPI,
	roomID, userID string,
	recentEvents []*gomatrixserverlib.HeaderedEvent,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	// We need to make sure we always include the latest states events, if they are in the timeline.
	// We grep at least limit * 2 events, to ensure we really get the needed events.
	filter := gomatrixserverlib.DefaultStateFilter()
	stateEvents, err := snapshot.CurrentState(ctx, roomID, &filter, nil)
	if err != nil {
		// Not a fatal error, we can continue without the stateEvents,
		// they are only needed if there are state events in the timeline.
		logrus.WithError(err).Warnf("Failed to get current room state for history visibility")
	}
	alwaysIncludeIDs := make(map[string]struct{}, len(stateEvents))
	for _, ev := range stateEvents {
		alwaysIncludeIDs[ev.EventID()] = struct{}{}
	}
	startTime := time.Now()
	events, err := internal.ApplyHistoryVisibilityFilter(ctx, snapshot, rsAPI, recentEvents, alwaysIncludeIDs, userID, "sync")
	if err != nil {
		return nil, err
	}
	logrus.WithFields(logrus.Fields{
		"duration": time.Since(startTime),
		"room_id":  roomID,
		"before":   len(recentEvents),
		"after":    len(events),
	}).Trace("Applied history visibility (sync)")
	return events, nil
}

func (p *PDUStreamProvider) addRoomSummary(ctx context.Context, snapshot storage.DatabaseTransaction, jr *types.JoinResponse, roomID, userID string, latestPosition types.StreamPosition) {
	// Work out how many members are in the room.
	joinedCount, _ := snapshot.MembershipCount(ctx, roomID, gomatrixserverlib.Join, latestPosition)
	invitedCount, _ := snapshot.MembershipCount(ctx, roomID, gomatrixserverlib.Invite, latestPosition)

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
	heroes, err := snapshot.GetRoomHeroes(ctx, roomID, userID, []string{"join", "invite"})
	if err != nil {
		return
	}
	sort.Strings(heroes)
	jr.Summary.Heroes = heroes
}

func (p *PDUStreamProvider) getJoinResponseForCompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
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
	recentStreamEvents, limited, err := snapshot.RecentEvents(
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

	stateEvents, err := snapshot.CurrentState(ctx, roomID, stateFilter, excludingEventIDs)
	if err != nil {
		return
	}

	p.addRoomSummary(ctx, snapshot, jr, roomID, device.UserID, r.From)

	// We don't include a device here as we don't need to send down
	// transaction IDs for complete syncs, but we do it anyway because Sytest demands it for:
	// "Can sync a room with a message with a transaction id" - which does a complete sync to check.
	recentEvents := snapshot.StreamEventsToEvents(device, recentStreamEvents)

	events := recentEvents
	// Only apply history visibility checks if the response is for joined rooms
	if !isPeek {
		events, err = applyHistoryVisibilityFilter(ctx, snapshot, p.rsAPI, roomID, device.UserID, recentEvents)
		if err != nil {
			logrus.WithError(err).Error("unable to apply history visibility filter")
		}
	}

	// If we are limited by the filter AND the history visibility filter
	// didn't "remove" events, return that the response is limited.
	limited = limited && len(events) == len(recentEvents)
	stateEvents = removeDuplicates(stateEvents, events)
	if stateFilter.LazyLoadMembers {
		if err != nil {
			return nil, err
		}
		stateEvents, err = p.lazyLoadMembers(
			ctx, snapshot, roomID,
			false, limited, stateFilter,
			device, recentEvents, stateEvents,
		)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
	}

	// Retrieve the backward topology position, i.e. the position of the
	// oldest event in the room's topology.
	var prevBatch *types.TopologyToken
	if len(events) > 0 {
		var backwardTopologyPos, backwardStreamPos types.StreamPosition
		event := events[0]
		// If this is the beginning of the room, we can't go back further. We're going to return
		// the TopologyToken from the last event instead. (Synapse returns the /sync next_Batch)
		if event.Type() == gomatrixserverlib.MRoomCreate && event.StateKeyEquals("") {
			event = events[len(events)-1]
		}
		backwardTopologyPos, backwardStreamPos, err = snapshot.PositionInTopology(ctx, event.EventID())
		if err != nil {
			return
		}
		prevBatch = &types.TopologyToken{
			Depth:       backwardTopologyPos,
			PDUPosition: backwardStreamPos,
		}
		prevBatch.Decrement()
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
	ctx context.Context, snapshot storage.DatabaseTransaction, roomID string,
	incremental, limited bool, stateFilter *gomatrixserverlib.StateFilter,
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
			if _, ok := timelineUsers[stateKey]; ok || isGappedIncremental || stateKey == device.UserID {
				newStateEvents = append(newStateEvents, event)
				if !stateFilter.IncludeRedundantMembers {
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
	memberships, err := snapshot.GetStateEventsForRoom(ctx, roomID, &filter)
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
func (p *PDUStreamProvider) addIgnoredUsersToFilter(ctx context.Context, snapshot storage.DatabaseTransaction, req *types.SyncRequest, eventFilter *gomatrixserverlib.RoomEventFilter) error {
	ignores, err := snapshot.IgnoresForUser(ctx, req.Device.UserID)
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
