package streams

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
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
	joinedRoomIDs, err := snapshot.RoomIDsWithMembership(ctx, req.Device.UserID, spec.Join)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.RoomIDsWithMembership failed")
		return from
	}

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if err = p.addIgnoredUsersToFilter(ctx, snapshot, req, &eventFilter); err != nil {
		req.Log.WithError(err).Error("unable to update event filter with ignored users")
	}

	eventFormat := synctypes.FormatSync
	if req.Filter.EventFormat == synctypes.EventFormatFederation {
		eventFormat = synctypes.FormatSyncFederation
	}

	recentEvents, err := snapshot.RecentEvents(ctx, joinedRoomIDs, r, &eventFilter, true, true)
	if err != nil {
		return from
	}
	// Build up a /sync response. Add joined rooms.
	for _, roomID := range joinedRoomIDs {
		events := recentEvents[roomID]
		// Invalidate the lazyLoadCache, otherwise we end up with missing displaynames/avatars
		// TODO: This might be inefficient, when joined to many and/or large rooms.
		joinedUsers := p.notifier.JoinedUsers(roomID)
		for _, sharedUser := range joinedUsers {
			p.lazyLoadCache.InvalidateLazyLoadedUser(req.Device, roomID, sharedUser)
		}

		// get the join response for each room
		jr, jerr := p.getJoinResponseForCompleteSync(
			ctx, snapshot, roomID, &stateFilter, req.WantFullState, req.Device, false,
			events.Events, events.Limited, eventFormat,
		)
		if jerr != nil {
			req.Log.WithError(jerr).Error("p.getJoinResponseForCompleteSync failed")
			if ctxErr := req.Context.Err(); ctxErr != nil || jerr == sql.ErrTxDone {
				return from
			}
			continue
		}
		req.Response.Rooms.Join[roomID] = jr
		req.Rooms[roomID] = spec.Join
	}

	// Add peeked rooms.
	peeks, err := snapshot.PeeksInRange(ctx, req.Device.UserID, req.Device.ID, r)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.PeeksInRange failed")
		return from
	}
	if len(peeks) > 0 {
		peekRooms := make([]string, 0, len(peeks))
		for _, peek := range peeks {
			if !peek.Deleted {
				peekRooms = append(peekRooms, peek.RoomID)
			}
		}

		recentEvents, err = snapshot.RecentEvents(ctx, peekRooms, r, &eventFilter, true, true)
		if err != nil {
			return from
		}

		for _, roomID := range peekRooms {
			var jr *types.JoinResponse
			events := recentEvents[roomID]
			jr, err = p.getJoinResponseForCompleteSync(
				ctx, snapshot, roomID, &stateFilter, req.WantFullState, req.Device, true,
				events.Events, events.Limited, eventFormat,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
				if err == context.DeadlineExceeded || err == context.Canceled || err == sql.ErrTxDone {
					return from
				}
				continue
			}
			req.Response.Rooms.Peek[roomID] = jr
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
		if stateDeltas, syncJoinedRooms, err = snapshot.GetStateDeltasForFullStateSync(ctx, req.Device, r, req.Device.UserID, &stateFilter, p.rsAPI); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltasForFullStateSync failed")
			return from
		}
	} else {
		if stateDeltas, syncJoinedRooms, err = snapshot.GetStateDeltas(ctx, req.Device, r, req.Device.UserID, &stateFilter, p.rsAPI); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltas failed")
			return from
		}
	}

	for _, roomID := range syncJoinedRooms {
		req.Rooms[roomID] = spec.Join
	}

	if len(stateDeltas) == 0 {
		return to
	}

	if err = p.addIgnoredUsersToFilter(ctx, snapshot, req, &eventFilter); err != nil {
		req.Log.WithError(err).Error("unable to update event filter with ignored users")
	}

	dbEvents, err := p.getRecentEvents(ctx, stateDeltas, r, eventFilter, &stateFilter, req, snapshot)
	if err != nil {
		req.Log.WithError(err).Error("unable to get recent events")
		return r.From
	}
	if len(dbEvents) == 0 {
		return r.To
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
		if pos, err = p.addRoomDeltaToResponse(ctx, snapshot, req.Device, newRange, delta, &eventFilter, &stateFilter, req, dbEvents); err != nil {
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

func (p *PDUStreamProvider) getRecentEvents(ctx context.Context, stateDeltas []types.StateDelta, r types.Range, eventFilter synctypes.RoomEventFilter, stateFilter *synctypes.StateFilter, req *types.SyncRequest, snapshot storage.DatabaseTransaction) (map[string]types.RecentEvents, error) {
	var roomIDs []string
	var newlyJoinedRoomIDs []string
	for _, delta := range stateDeltas {
		if delta.NewlyJoined {
			newlyJoinedRoomIDs = append(newlyJoinedRoomIDs, delta.RoomID)
		} else {
			roomIDs = append(roomIDs, delta.RoomID)
		}
	}
	dbEvents := make(map[string]types.RecentEvents)
	if len(roomIDs) > 0 {
		events, err := snapshot.RecentEvents(
			ctx, roomIDs, r,
			&eventFilter, true, true,
		)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, err
			}
		}
		for k, v := range events {
			dbEvents[k] = v
		}
	}
	if len(newlyJoinedRoomIDs) > 0 {
		// For rooms that were joined in this sync, try to fetch
		// as much timeline events as allowed by the filter.

		filter := eventFilter
		// If we're going backwards, grep at least X events, this is mostly to satisfy Sytest
		if eventFilter.Limit < recentEventBackwardsLimit {
			filter.Limit = recentEventBackwardsLimit // TODO: Figure out a better way
			diff := r.From - r.To
			if diff > 0 && diff < recentEventBackwardsLimit {
				filter.Limit = int(diff)
			}
		}

		events, err := snapshot.RecentEvents(
			ctx, newlyJoinedRoomIDs, types.Range{
				From:      r.To,
				To:        0,
				Backwards: true,
			},
			&filter, true, true,
		)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, err
			}
		}
		for k, v := range events {
			dbEvents[k] = v
		}
	}

	return dbEvents, nil
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
	eventFilter *synctypes.RoomEventFilter,
	stateFilter *synctypes.StateFilter,
	req *types.SyncRequest,
	dbEvents map[string]types.RecentEvents,
) (types.StreamPosition, error) {
	var err error
	recentStreamEvents := dbEvents[delta.RoomID].Events
	limited := dbEvents[delta.RoomID].Limited

	recEvents := gomatrixserverlib.ReverseTopologicalOrdering(
		gomatrixserverlib.ToPDUs(snapshot.StreamEventsToEvents(ctx, device, recentStreamEvents, p.rsAPI)),
		gomatrixserverlib.TopologicalOrderByPrevEvents,
	)
	recentEvents := make([]*rstypes.HeaderedEvent, len(recEvents))
	for i := range recEvents {
		recentEvents[i] = recEvents[i].(*rstypes.HeaderedEvent)
	}

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
		if recentEvent.Type() == spec.MRoomMember && recentEvent.StateKey() != nil {
			if membership, _ := recentEvent.Membership(); membership == spec.Join {
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

	if r.Backwards && len(events) > eventFilter.Limit {
		// We're going backwards and the events are ordered chronologically, so take the last `limit` events
		events = events[len(events)-eventFilter.Limit:]
		limited = true
	}

	prevBatch, err := snapshot.GetBackwardTopologyPos(ctx, events)
	if err != nil {
		return r.From, fmt.Errorf("p.DB.GetBackwardTopologyPos: %w", err)
	}

	eventFormat := synctypes.FormatSync
	if req.Filter.EventFormat == synctypes.EventFormatFederation {
		eventFormat = synctypes.FormatSyncFederation
	}

	// Now that we've filtered the timeline, work out which state events are still
	// left. Anything that appears in the filtered timeline will be removed from the
	// "state" section and kept in "timeline".
	sEvents := gomatrixserverlib.HeaderedReverseTopologicalOrdering(
		gomatrixserverlib.ToPDUs(removeDuplicates(delta.StateEvents, events)),
		gomatrixserverlib.TopologicalOrderByAuthEvents,
	)
	delta.StateEvents = make([]*rstypes.HeaderedEvent, len(sEvents))
	var skipped int
	for i := range sEvents {
		ev := sEvents[i]
		he, ok := ev.(*rstypes.HeaderedEvent)
		if !ok {
			skipped++
			continue
		}
		delta.StateEvents[i-skipped] = he
	}
	delta.StateEvents = delta.StateEvents[:len(sEvents)-skipped]

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
	case spec.Join:
		jr := types.NewJoinResponse()
		if hasMembershipChange {
			jr.Summary, err = snapshot.GetRoomSummary(ctx, delta.RoomID, device.UserID)
			if err != nil {
				logrus.WithError(err).Warn("failed to get room summary")
			}
		}
		jr.Timeline.PrevBatch = &prevBatch
		jr.Timeline.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(events), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		})
		// If we are limited by the filter AND the history visibility filter
		// didn't "remove" events, return that the response is limited.
		jr.Timeline.Limited = (limited && len(events) == len(recentEvents)) || delta.NewlyJoined
		jr.State.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(delta.StateEvents), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		})
		req.Response.Rooms.Join[delta.RoomID] = jr

	case spec.Peek:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = &prevBatch
		// TODO: Apply history visibility on peeked rooms
		jr.Timeline.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(recentEvents), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		})
		jr.Timeline.Limited = limited
		jr.State.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(delta.StateEvents), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		})
		req.Response.Rooms.Peek[delta.RoomID] = jr

	case spec.Leave:
		fallthrough // transitions to leave are the same as ban

	case spec.Ban:
		lr := types.NewLeaveResponse()
		lr.Timeline.PrevBatch = &prevBatch
		lr.Timeline.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(events), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		})
		// If we are limited by the filter AND the history visibility filter
		// didn't "remove" events, return that the response is limited.
		lr.Timeline.Limited = limited && len(events) == len(recentEvents)
		lr.State.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(delta.StateEvents), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		})
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
	recentEvents []*rstypes.HeaderedEvent,
) ([]*rstypes.HeaderedEvent, error) {
	// We need to make sure we always include the latest state events, if they are in the timeline.
	alwaysIncludeIDs := make(map[string]struct{})
	var stateTypes []string
	var senders []string
	for _, ev := range recentEvents {
		if ev.StateKey() != nil {
			stateTypes = append(stateTypes, ev.Type())
			senders = append(senders, string(ev.SenderID()))
		}
	}

	// Only get the state again if there are state events in the timeline
	if len(stateTypes) > 0 {
		filter := synctypes.DefaultStateFilter()
		filter.Types = &stateTypes
		filter.Senders = &senders
		stateEvents, err := snapshot.CurrentState(ctx, roomID, &filter, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get current room state for history visibility calculation: %w", err)
		}

		for _, ev := range stateEvents {
			alwaysIncludeIDs[ev.EventID()] = struct{}{}
		}
	}

	parsedUserID, err := spec.NewUserID(userID, true)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	events, err := internal.ApplyHistoryVisibilityFilter(ctx, snapshot, rsAPI, recentEvents, alwaysIncludeIDs, *parsedUserID, "sync")
	if err != nil {
		return nil, err
	}
	logrus.WithFields(logrus.Fields{
		"duration": time.Since(startTime),
		"room_id":  roomID,
		"before":   len(recentEvents),
		"after":    len(events),
	}).Debugf("Applied history visibility (sync)")
	return events, nil
}

// nolint: gocyclo
func (p *PDUStreamProvider) getJoinResponseForCompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	roomID string,
	stateFilter *synctypes.StateFilter,
	wantFullState bool,
	device *userapi.Device,
	isPeek bool,
	recentStreamEvents []types.StreamEvent,
	limited bool,
	eventFormat synctypes.ClientEventFormat,
) (jr *types.JoinResponse, err error) {
	jr = types.NewJoinResponse()
	// TODO: When filters are added, we may need to call this multiple times to get enough events.
	//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316

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
		return jr, err
	}

	jr.Summary, err = snapshot.GetRoomSummary(ctx, roomID, device.UserID)
	if err != nil {
		logrus.WithError(err).Warn("failed to get room summary")
	}

	// We don't include a device here as we don't need to send down
	// transaction IDs for complete syncs, but we do it anyway because Sytest demands it for:
	// "Can sync a room with a message with a transaction id" - which does a complete sync to check.
	recentEvents := snapshot.StreamEventsToEvents(ctx, device, recentStreamEvents, p.rsAPI)

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
		if event.Type() == spec.MRoomCreate && event.StateKeyEquals("") {
			event = events[len(events)-1]
		}
		backwardTopologyPos, backwardStreamPos, err = snapshot.PositionInTopology(ctx, event.EventID())
		if err != nil {
			return jr, err
		}
		prevBatch = &types.TopologyToken{
			Depth:       backwardTopologyPos,
			PDUPosition: backwardStreamPos,
		}
		prevBatch.Decrement()
	}

	jr.Timeline.PrevBatch = prevBatch
	jr.Timeline.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(events), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
		return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
	})
	// If we are limited by the filter AND the history visibility filter
	// didn't "remove" events, return that the response is limited.
	jr.Timeline.Limited = limited && len(events) == len(recentEvents)
	jr.State.Events = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(stateEvents), eventFormat, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
		return p.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
	})
	return jr, nil
}

func (p *PDUStreamProvider) lazyLoadMembers(
	ctx context.Context, snapshot storage.DatabaseTransaction, roomID string,
	incremental, limited bool, stateFilter *synctypes.StateFilter,
	device *userapi.Device,
	timelineEvents, stateEvents []*rstypes.HeaderedEvent,
) ([]*rstypes.HeaderedEvent, error) {
	if len(timelineEvents) == 0 {
		return stateEvents, nil
	}
	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return nil, err
	}
	// Work out which memberships to include
	timelineUsers := make(map[string]struct{})
	if !incremental {
		timelineUsers[device.UserID] = struct{}{}
	}
	// Add all users the client doesn't know about yet to a list
	for _, event := range timelineEvents {
		// Membership is not yet cached, add it to the list
		if _, ok := p.lazyLoadCache.IsLazyLoadedUserCached(device, roomID, string(event.SenderID())); !ok {
			timelineUsers[string(event.SenderID())] = struct{}{}
		}
	}
	// Preallocate with the same amount, even if it will end up with fewer values
	newStateEvents := make([]*rstypes.HeaderedEvent, 0, len(stateEvents))
	// Remove existing membership events we don't care about, e.g. users not in the timeline.events
	for _, event := range stateEvents {
		if event.Type() == spec.MRoomMember && event.StateKey() != nil {
			// If this is a gapped incremental sync, we still want this membership
			isGappedIncremental := limited && incremental
			// We want this users membership event, keep it in the list
			userID := ""
			stateKeyUserID, queryErr := p.rsAPI.QueryUserIDForSender(ctx, *validRoomID, spec.SenderID(*event.StateKey()))
			if queryErr == nil && stateKeyUserID != nil {
				userID = stateKeyUserID.String()
			}
			if _, ok := timelineUsers[userID]; ok || isGappedIncremental || userID == device.UserID {
				newStateEvents = append(newStateEvents, event)
				if !stateFilter.IncludeRedundantMembers {
					p.lazyLoadCache.StoreLazyLoadedUser(device, roomID, userID, event.EventID())
				}
				delete(timelineUsers, userID)
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
	filter := synctypes.DefaultStateFilter()
	filter.Senders = &wantUsers
	filter.Types = &[]string{spec.MRoomMember}
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
func (p *PDUStreamProvider) addIgnoredUsersToFilter(ctx context.Context, snapshot storage.DatabaseTransaction, req *types.SyncRequest, eventFilter *synctypes.RoomEventFilter) error {
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

func removeDuplicates[T gomatrixserverlib.PDU](stateEvents, recentEvents []T) []T {
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
