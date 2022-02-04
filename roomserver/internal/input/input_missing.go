package input

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	fedapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type missingStateReq struct {
	origin          gomatrixserverlib.ServerName
	db              *shared.RoomUpdater
	inputer         *Inputer
	queryer         *query.Queryer
	keys            gomatrixserverlib.JSONVerifier
	federation      fedapi.FederationInternalAPI
	roomsMu         *internal.MutexByRoom
	servers         []gomatrixserverlib.ServerName
	hadEvents       map[string]bool
	hadEventsMutex  sync.Mutex
	haveEvents      map[string]*gomatrixserverlib.HeaderedEvent
	haveEventsMutex sync.Mutex
}

// processEventWithMissingState is the entrypoint for a missingStateReq
// request, as called from processRoomEvent.
func (t *missingStateReq) processEventWithMissingState(
	ctx context.Context, e *gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion,
) error {
	// We are missing the previous events for this events.
	// This means that there is a gap in our view of the history of the
	// room. There two ways that we can handle such a gap:
	//   1) We can fill in the gap using /get_missing_events
	//   2) We can leave the gap and request the state of the room at
	//      this event from the remote server using either /state_ids
	//      or /state.
	// Synapse will attempt to do 1 and if that fails or if the gap is
	// too large then it will attempt 2.
	// Synapse will use /state_ids if possible since usually the state
	// is largely unchanged and it is more efficient to fetch a list of
	// event ids and then use /event to fetch the individual events.
	// However not all version of synapse support /state_ids so you may
	// need to fallback to /state.
	logger := util.GetLogger(ctx).WithFields(map[string]interface{}{
		"txn_event":       e.EventID(),
		"room_id":         e.RoomID(),
		"txn_prev_events": e.PrevEventIDs(),
	})

	// Attempt to fill in the gap using /get_missing_events
	// This will either:
	// - fill in the gap completely then process event `e` returning no backwards extremity
	// - fail to fill in the gap and tell us to terminate the transaction err=not nil
	// - fail to fill in the gap and tell us to fetch state at the new backwards extremity, and to not terminate the transaction
	newEvents, isGapFilled, err := t.getMissingEvents(ctx, e, roomVersion)
	if err != nil {
		return fmt.Errorf("t.getMissingEvents: %w", err)
	}
	if len(newEvents) == 0 {
		return fmt.Errorf("expected to find missing events but didn't")
	}
	if isGapFilled {
		logger.Infof("gap filled by /get_missing_events, injecting %d new events", len(newEvents))
		// we can just inject all the newEvents as new as we may have only missed 1 or 2 events and have filled
		// in the gap in the DAG
		for _, newEvent := range newEvents {
			_, err = t.inputer.processRoomEvent(ctx, t.db, &api.InputRoomEvent{
				Kind:         api.KindNew,
				Event:        newEvent.Headered(roomVersion),
				Origin:       t.origin,
				SendAsServer: api.DoNotSendToOtherServers,
			})
			if err != nil {
				return fmt.Errorf("t.inputer.processRoomEvent: %w", err)
			}
		}
		return nil
	}

	backwardsExtremity := newEvents[0]
	newEvents = newEvents[1:]

	type respState struct {
		// A snapshot is considered trustworthy if it came from our own roomserver.
		// That's because the state will have been through state resolution once
		// already in QueryStateAfterEvent.
		trustworthy bool
		*gomatrixserverlib.RespState
	}

	// at this point we know we're going to have a gap: we need to work out the room state at the new backwards extremity.
	// Therefore, we cannot just query /state_ids with this event to get the state before. Instead, we need to query
	// the state AFTER all the prev_events for this event, then apply state resolution to that to get the state before the event.
	var states []*respState
	for _, prevEventID := range backwardsExtremity.PrevEventIDs() {
		// Look up what the state is after the backward extremity. This will either
		// come from the roomserver, if we know all the required events, or it will
		// come from a remote server via /state_ids if not.
		prevState, trustworthy, lerr := t.lookupStateAfterEvent(ctx, roomVersion, backwardsExtremity.RoomID(), prevEventID)
		if lerr != nil {
			logger.WithError(lerr).Errorf("Failed to lookup state after prev_event: %s", prevEventID)
			return lerr
		}
		// Append the state onto the collected state. We'll run this through the
		// state resolution next.
		states = append(states, &respState{trustworthy, prevState})
	}

	// Now that we have collected all of the state from the prev_events, we'll
	// run the state through the appropriate state resolution algorithm for the
	// room if needed. This does a couple of things:
	// 1. Ensures that the state is deduplicated fully for each state-key tuple
	// 2. Ensures that we pick the latest events from both sets, in the case that
	//    one of the prev_events is quite a bit older than the others
	resolvedState := &gomatrixserverlib.RespState{}
	switch len(states) {
	case 0:
		extremityIsCreate := backwardsExtremity.Type() == gomatrixserverlib.MRoomCreate && backwardsExtremity.StateKeyEquals("")
		if !extremityIsCreate {
			// There are no previous states and this isn't the beginning of the
			// room - this is an error condition!
			logger.Errorf("Failed to lookup any state after prev_events")
			return fmt.Errorf("expected %d states but got %d", len(backwardsExtremity.PrevEventIDs()), len(states))
		}
	case 1:
		// There's only one previous state - if it's trustworthy (came from a
		// local state snapshot which will already have been through state res),
		// use it as-is. There's no point in resolving it again.
		if states[0].trustworthy {
			resolvedState = states[0].RespState
			break
		}
		// Otherwise, if it isn't trustworthy (came from federation), run it through
		// state resolution anyway for safety, in case there are duplicates.
		fallthrough
	default:
		respStates := make([]*gomatrixserverlib.RespState, len(states))
		for i := range states {
			respStates[i] = states[i].RespState
		}
		// There's more than one previous state - run them all through state res
		t.roomsMu.Lock(e.RoomID())
		resolvedState, err = t.resolveStatesAndCheck(ctx, roomVersion, respStates, backwardsExtremity)
		t.roomsMu.Unlock(e.RoomID())
		if err != nil {
			logger.WithError(err).Errorf("Failed to resolve state conflicts for event %s", backwardsExtremity.EventID())
			return err
		}
	}

	hadEvents := map[string]bool{}
	t.hadEventsMutex.Lock()
	for k, v := range t.hadEvents {
		hadEvents[k] = v
	}
	t.hadEventsMutex.Unlock()

	// Send outliers first so we can send the new backwards extremity without causing errors
	outliers, err := resolvedState.Events()
	if err != nil {
		return err
	}
	var outlierRoomEvents []api.InputRoomEvent
	for _, outlier := range outliers {
		if hadEvents[outlier.EventID()] {
			continue
		}
		outlierRoomEvents = append(outlierRoomEvents, api.InputRoomEvent{
			Kind:   api.KindOutlier,
			Event:  outlier.Headered(roomVersion),
			Origin: t.origin,
		})
	}
	// TODO: we could do this concurrently?
	for _, ire := range outlierRoomEvents {
		if _, err = t.inputer.processRoomEvent(ctx, t.db, &ire); err != nil {
			return fmt.Errorf("t.inputer.processRoomEvent[outlier]: %w", err)
		}
	}

	// Now send the backward extremity into the roomserver with the
	// newly resolved state. This marks the "oldest" point in the backfill and
	// sets the baseline state for any new events after this.
	stateIDs := make([]string, 0, len(resolvedState.StateEvents))
	for _, event := range resolvedState.StateEvents {
		stateIDs = append(stateIDs, event.EventID())
	}

	_, err = t.inputer.processRoomEvent(ctx, t.db, &api.InputRoomEvent{
		Kind:          api.KindOld,
		Event:         backwardsExtremity.Headered(roomVersion),
		Origin:        t.origin,
		HasState:      true,
		StateEventIDs: stateIDs,
		SendAsServer:  api.DoNotSendToOtherServers,
	})
	if err != nil {
		return fmt.Errorf("t.inputer.processRoomEvent: %w", err)
	}

	// Then send all of the newer backfilled events, of which will all be newer
	// than the backward extremity, into the roomserver without state. This way
	// they will automatically fast-forward based on the room state at the
	// extremity in the last step.
	for _, newEvent := range newEvents {
		_, err = t.inputer.processRoomEvent(ctx, t.db, &api.InputRoomEvent{
			Kind:         api.KindOld,
			Event:        newEvent.Headered(roomVersion),
			Origin:       t.origin,
			SendAsServer: api.DoNotSendToOtherServers,
		})
		if err != nil {
			return fmt.Errorf("t.inputer.processRoomEvent: %w", err)
		}
	}

	return nil
}

// lookupStateAfterEvent returns the room state after `eventID`, which is the state before eventID with the state of `eventID` (if it's a state event)
// added into the mix.
func (t *missingStateReq) lookupStateAfterEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string) (*gomatrixserverlib.RespState, bool, error) {
	// try doing all this locally before we resort to querying federation
	respState := t.lookupStateAfterEventLocally(ctx, roomID, eventID)
	if respState != nil {
		return respState, true, nil
	}

	respState, err := t.lookupStateBeforeEvent(ctx, roomVersion, roomID, eventID)
	if err != nil {
		return nil, false, fmt.Errorf("t.lookupStateBeforeEvent: %w", err)
	}

	// fetch the event we're missing and add it to the pile
	h, err := t.lookupEvent(ctx, roomVersion, roomID, eventID, false)
	switch err.(type) {
	case verifySigError:
		return respState, false, nil
	case nil:
		// do nothing
	default:
		return nil, false, fmt.Errorf("t.lookupEvent: %w", err)
	}
	h = t.cacheAndReturn(h)
	if h.StateKey() != nil {
		addedToState := false
		for i := range respState.StateEvents {
			se := respState.StateEvents[i]
			if se.Type() == h.Type() && se.StateKeyEquals(*h.StateKey()) {
				respState.StateEvents[i] = h.Unwrap()
				addedToState = true
				break
			}
		}
		if !addedToState {
			respState.StateEvents = append(respState.StateEvents, h.Unwrap())
		}
	}

	return respState, false, nil
}

func (t *missingStateReq) cacheAndReturn(ev *gomatrixserverlib.HeaderedEvent) *gomatrixserverlib.HeaderedEvent {
	t.haveEventsMutex.Lock()
	defer t.haveEventsMutex.Unlock()
	if cached, exists := t.haveEvents[ev.EventID()]; exists {
		return cached
	}
	t.haveEvents[ev.EventID()] = ev
	return ev
}

func (t *missingStateReq) lookupStateAfterEventLocally(ctx context.Context, roomID, eventID string) *gomatrixserverlib.RespState {
	var res api.QueryStateAfterEventsResponse
	err := t.queryer.QueryStateAfterEvents(ctx, &api.QueryStateAfterEventsRequest{
		RoomID:       roomID,
		PrevEventIDs: []string{eventID},
	}, &res)
	if err != nil || !res.PrevEventsExist {
		util.GetLogger(ctx).WithField("room_id", roomID).WithError(err).Warnf("failed to query state after %s locally, prev exists=%v", eventID, res.PrevEventsExist)
		return nil
	}
	stateEvents := make([]*gomatrixserverlib.HeaderedEvent, len(res.StateEvents))
	for i, ev := range res.StateEvents {
		// set the event from the haveEvents cache - this means we will share pointers with other prev_event branches for this
		// processEvent request, which is better for memory.
		stateEvents[i] = t.cacheAndReturn(ev)
		t.hadEvent(ev.EventID())
	}
	// we should never access res.StateEvents again so we delete it here to make GC faster
	res.StateEvents = nil

	var authEvents []*gomatrixserverlib.Event
	missingAuthEvents := map[string]bool{}
	for _, ev := range stateEvents {
		t.haveEventsMutex.Lock()
		for _, ae := range ev.AuthEventIDs() {
			if aev, ok := t.haveEvents[ae]; ok {
				authEvents = append(authEvents, aev.Unwrap())
			} else {
				missingAuthEvents[ae] = true
			}
		}
		t.haveEventsMutex.Unlock()
	}
	// QueryStateAfterEvents does not return the auth events, so fetch them now. We know the roomserver has them else it wouldn't
	// have stored the event.
	if len(missingAuthEvents) > 0 {
		var missingEventList []string
		for evID := range missingAuthEvents {
			missingEventList = append(missingEventList, evID)
		}
		queryReq := api.QueryEventsByIDRequest{
			EventIDs: missingEventList,
		}
		util.GetLogger(ctx).WithField("count", len(missingEventList)).Debugf("Fetching missing auth events")
		var queryRes api.QueryEventsByIDResponse
		if err = t.queryer.QueryEventsByID(ctx, &queryReq, &queryRes); err != nil {
			return nil
		}
		for i, ev := range queryRes.Events {
			authEvents = append(authEvents, t.cacheAndReturn(queryRes.Events[i]).Unwrap())
			t.hadEvent(ev.EventID())
		}
		queryRes.Events = nil
	}

	return &gomatrixserverlib.RespState{
		StateEvents: gomatrixserverlib.UnwrapEventHeaders(stateEvents),
		AuthEvents:  authEvents,
	}
}

// lookuptStateBeforeEvent returns the room state before the event e, which is just /state_ids and/or /state depending on what
// the server supports.
func (t *missingStateReq) lookupStateBeforeEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string) (
	*gomatrixserverlib.RespState, error) {

	// Attempt to fetch the missing state using /state_ids and /events
	return t.lookupMissingStateViaStateIDs(ctx, roomID, eventID, roomVersion)
}

func (t *missingStateReq) resolveStatesAndCheck(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, states []*gomatrixserverlib.RespState, backwardsExtremity *gomatrixserverlib.Event) (*gomatrixserverlib.RespState, error) {
	var authEventList []*gomatrixserverlib.Event
	var stateEventList []*gomatrixserverlib.Event
	for _, state := range states {
		authEventList = append(authEventList, state.AuthEvents...)
		stateEventList = append(stateEventList, state.StateEvents...)
	}
	resolvedStateEvents, err := gomatrixserverlib.ResolveConflicts(roomVersion, stateEventList, authEventList)
	if err != nil {
		return nil, err
	}
	// apply the current event
retryAllowedState:
	if err = checkAllowedByState(backwardsExtremity, resolvedStateEvents); err != nil {
		switch missing := err.(type) {
		case gomatrixserverlib.MissingAuthEventError:
			h, err2 := t.lookupEvent(ctx, roomVersion, backwardsExtremity.RoomID(), missing.AuthEventID, true)
			switch err2.(type) {
			case verifySigError:
				return &gomatrixserverlib.RespState{
					AuthEvents:  authEventList,
					StateEvents: resolvedStateEvents,
				}, nil
			case nil:
				// do nothing
			default:
				return nil, fmt.Errorf("missing auth event %s and failed to look it up: %w", missing.AuthEventID, err2)
			}
			util.GetLogger(ctx).Tracef("fetched event %s", missing.AuthEventID)
			resolvedStateEvents = append(resolvedStateEvents, h.Unwrap())
			goto retryAllowedState
		default:
		}
		return nil, err
	}
	return &gomatrixserverlib.RespState{
		AuthEvents:  authEventList,
		StateEvents: resolvedStateEvents,
	}, nil
}

// get missing events for `e`. If `isGapFilled`=true then `newEvents` contains all the events to inject,
// without `e`. If `isGapFilled=false` then `newEvents` contains the response to /get_missing_events
func (t *missingStateReq) getMissingEvents(ctx context.Context, e *gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion) (newEvents []*gomatrixserverlib.Event, isGapFilled bool, err error) {
	logger := util.GetLogger(ctx).WithField("event_id", e.EventID()).WithField("room_id", e.RoomID())
	needed := gomatrixserverlib.StateNeededForAuth([]*gomatrixserverlib.Event{e})
	// query latest events (our trusted forward extremities)
	req := api.QueryLatestEventsAndStateRequest{
		RoomID:       e.RoomID(),
		StateToFetch: needed.Tuples(),
	}
	var res api.QueryLatestEventsAndStateResponse
	if err = t.queryer.QueryLatestEventsAndState(ctx, &req, &res); err != nil {
		logger.WithError(err).Warn("Failed to query latest events")
		return nil, false, err
	}
	latestEvents := make([]string, len(res.LatestEvents))
	for i, ev := range res.LatestEvents {
		latestEvents[i] = res.LatestEvents[i].EventID
		t.hadEvent(ev.EventID)
	}

	var missingResp *gomatrixserverlib.RespMissingEvents
	for _, server := range t.servers {
		var m gomatrixserverlib.RespMissingEvents
		if m, err = t.federation.LookupMissingEvents(ctx, server, e.RoomID(), gomatrixserverlib.MissingEvents{
			Limit: 20,
			// The latest event IDs that the sender already has. These are skipped when retrieving the previous events of latest_events.
			EarliestEvents: latestEvents,
			// The event IDs to retrieve the previous events for.
			LatestEvents: []string{e.EventID()},
		}, roomVersion); err == nil {
			missingResp = &m
			break
		} else {
			logger.WithError(err).Warnf("%s pushed us an event but %q did not respond to /get_missing_events", t.origin, server)
			if errors.Is(err, context.DeadlineExceeded) {
				select {
				case <-ctx.Done(): // the parent request context timed out
					return nil, false, context.DeadlineExceeded
				default: // this request exceed its own timeout
					continue
				}
			}
		}
	}

	if missingResp == nil {
		logger.WithError(err).Warnf(
			"%s pushed us an event but %d server(s) couldn't give us details about prev_events via /get_missing_events - dropping this event until it can",
			t.origin, len(t.servers),
		)
		return nil, false, missingPrevEventsError{
			eventID: e.EventID(),
			err:     err,
		}
	}

	// Make sure events from the missingResp are using the cache - missing events
	// will be added and duplicates will be removed.
	logger.Debugf("get_missing_events returned %d events", len(missingResp.Events))
	for i, ev := range missingResp.Events {
		missingResp.Events[i] = t.cacheAndReturn(ev.Headered(roomVersion)).Unwrap()
	}

	// topologically sort and sanity check that we are making forward progress
	newEvents = gomatrixserverlib.ReverseTopologicalOrdering(missingResp.Events, gomatrixserverlib.TopologicalOrderByPrevEvents)
	shouldHaveSomeEventIDs := e.PrevEventIDs()
	hasPrevEvent := false
Event:
	for _, pe := range shouldHaveSomeEventIDs {
		for _, ev := range newEvents {
			if ev.EventID() == pe {
				hasPrevEvent = true
				break Event
			}
		}
	}
	if !hasPrevEvent {
		err = fmt.Errorf("called /get_missing_events but server %s didn't return any prev_events with IDs %v", t.origin, shouldHaveSomeEventIDs)
		logger.WithError(err).Warnf(
			"%s pushed us an event but couldn't give us details about prev_events via /get_missing_events - dropping this event until it can",
			t.origin,
		)
		return nil, false, missingPrevEventsError{
			eventID: e.EventID(),
			err:     err,
		}
	}
	if len(newEvents) == 0 {
		return nil, false, nil // TODO: error instead?
	}

	// now check if we can fill the gap. Look to see if we have state snapshot IDs for the earliest event
	earliestNewEvent := newEvents[0]
	if state, err := t.db.StateAtEventIDs(ctx, []string{earliestNewEvent.EventID()}); err != nil || len(state) == 0 {
		if earliestNewEvent.Type() == gomatrixserverlib.MRoomCreate && earliestNewEvent.StateKeyEquals("") {
			// we got to the beginning of the room so there will be no state! It's all good we can process this
			return newEvents, true, nil
		}
		// we don't have the state at this earliest event from /g_m_e so we won't have state for later events either
		return newEvents, false, nil
	}
	// StateAtEventIDs returned some kind of state for the earliest event so we can fill in the gap!
	return newEvents, true, nil
}

func (t *missingStateReq) lookupMissingStateViaState(ctx context.Context, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
	respState *gomatrixserverlib.RespState, err error) {
	state, err := t.federation.LookupState(ctx, t.origin, roomID, eventID, roomVersion)
	if err != nil {
		return nil, err
	}
	// Check that the returned state is valid.
	if err := state.Check(ctx, t.keys, nil); err != nil {
		return nil, err
	}
	// Cache the results of this state lookup and deduplicate anything we already
	// have in the cache, freeing up memory.
	for i, ev := range state.AuthEvents {
		state.AuthEvents[i] = t.cacheAndReturn(ev.Headered(roomVersion)).Unwrap()
	}
	for i, ev := range state.StateEvents {
		state.StateEvents[i] = t.cacheAndReturn(ev.Headered(roomVersion)).Unwrap()
	}
	return &state, nil
}

func (t *missingStateReq) lookupMissingStateViaStateIDs(ctx context.Context, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
	*gomatrixserverlib.RespState, error) {
	util.GetLogger(ctx).WithField("room_id", roomID).Infof("lookupMissingStateViaStateIDs %s", eventID)
	// fetch the state event IDs at the time of the event
	stateIDs, err := t.federation.LookupStateIDs(ctx, t.origin, roomID, eventID)
	if err != nil {
		return nil, err
	}
	// work out which auth/state IDs are missing
	wantIDs := append(stateIDs.StateEventIDs, stateIDs.AuthEventIDs...)
	missing := make(map[string]bool)
	var missingEventList []string
	t.haveEventsMutex.Lock()
	for _, sid := range wantIDs {
		if _, ok := t.haveEvents[sid]; !ok {
			if !missing[sid] {
				missing[sid] = true
				missingEventList = append(missingEventList, sid)
			}
		}
	}
	t.haveEventsMutex.Unlock()

	// fetch as many as we can from the roomserver
	queryReq := api.QueryEventsByIDRequest{
		EventIDs: missingEventList,
	}
	var queryRes api.QueryEventsByIDResponse
	if err = t.queryer.QueryEventsByID(ctx, &queryReq, &queryRes); err != nil {
		return nil, err
	}
	for i, ev := range queryRes.Events {
		queryRes.Events[i] = t.cacheAndReturn(queryRes.Events[i])
		t.hadEvent(ev.EventID())
		evID := queryRes.Events[i].EventID()
		if missing[evID] {
			delete(missing, evID)
		}
	}
	queryRes.Events = nil // allow it to be GCed

	concurrentRequests := 8
	missingCount := len(missing)
	util.GetLogger(ctx).WithField("room_id", roomID).WithField("event_id", eventID).Debugf("lookupMissingStateViaStateIDs missing %d/%d events", missingCount, len(wantIDs))

	// If over 50% of the auth/state events from /state_ids are missing
	// then we'll just call /state instead, otherwise we'll just end up
	// hammering the remote side with /event requests unnecessarily.
	if missingCount > concurrentRequests && missingCount > len(wantIDs)/2 {
		util.GetLogger(ctx).WithFields(logrus.Fields{
			"missing":           missingCount,
			"event_id":          eventID,
			"room_id":           roomID,
			"total_state":       len(stateIDs.StateEventIDs),
			"total_auth_events": len(stateIDs.AuthEventIDs),
		}).Debug("Fetching all state at event")
		return t.lookupMissingStateViaState(ctx, roomID, eventID, roomVersion)
	}

	if missingCount > 0 {
		util.GetLogger(ctx).WithFields(logrus.Fields{
			"missing":             missingCount,
			"event_id":            eventID,
			"room_id":             roomID,
			"total_state":         len(stateIDs.StateEventIDs),
			"total_auth_events":   len(stateIDs.AuthEventIDs),
			"concurrent_requests": concurrentRequests,
		}).Debug("Fetching missing state at event")

		// Create a queue containing all of the missing event IDs that we want
		// to retrieve.
		pending := make(chan string, missingCount)
		for missingEventID := range missing {
			pending <- missingEventID
		}
		close(pending)

		// Define how many workers we should start to do this.
		if missingCount < concurrentRequests {
			concurrentRequests = missingCount
		}

		// Create the wait group.
		var fetchgroup sync.WaitGroup
		fetchgroup.Add(concurrentRequests)

		// This is the only place where we'll write to t.haveEvents from
		// multiple goroutines, and everywhere else is blocked on this
		// synchronous function anyway.
		var haveEventsMutex sync.Mutex

		// Define what we'll do in order to fetch the missing event ID.
		fetch := func(missingEventID string) {
			var h *gomatrixserverlib.HeaderedEvent
			h, err = t.lookupEvent(ctx, roomVersion, roomID, missingEventID, false)
			switch err.(type) {
			case verifySigError:
				return
			case nil:
				break
			default:
				util.GetLogger(ctx).WithFields(logrus.Fields{
					"event_id": missingEventID,
					"room_id":  roomID,
				}).Warn("Failed to fetch missing event")
				return
			}
			haveEventsMutex.Lock()
			t.cacheAndReturn(h)
			haveEventsMutex.Unlock()
		}

		// Create the worker.
		worker := func(ch <-chan string) {
			defer fetchgroup.Done()
			for missingEventID := range ch {
				fetch(missingEventID)
			}
		}

		// Start the workers.
		for i := 0; i < concurrentRequests; i++ {
			go worker(pending)
		}

		// Wait for the workers to finish.
		fetchgroup.Wait()
	}

	resp, err := t.createRespStateFromStateIDs(stateIDs)
	return resp, err
}

func (t *missingStateReq) createRespStateFromStateIDs(stateIDs gomatrixserverlib.RespStateIDs) (
	*gomatrixserverlib.RespState, error) { // nolint:unparam
	t.haveEventsMutex.Lock()
	defer t.haveEventsMutex.Unlock()

	// create a RespState response using the response to /state_ids as a guide
	respState := gomatrixserverlib.RespState{}

	for i := range stateIDs.StateEventIDs {
		ev, ok := t.haveEvents[stateIDs.StateEventIDs[i]]
		if !ok {
			logrus.Tracef("Missing state event in createRespStateFromStateIDs: %s", stateIDs.StateEventIDs[i])
			continue
		}
		respState.StateEvents = append(respState.StateEvents, ev.Unwrap())
	}
	for i := range stateIDs.AuthEventIDs {
		ev, ok := t.haveEvents[stateIDs.AuthEventIDs[i]]
		if !ok {
			logrus.Tracef("Missing auth event in createRespStateFromStateIDs: %s", stateIDs.AuthEventIDs[i])
			continue
		}
		respState.AuthEvents = append(respState.AuthEvents, ev.Unwrap())
	}
	// We purposefully do not do auth checks on the returned events, as they will still
	// be processed in the exact same way, just as a 'rejected' event
	// TODO: Add a field to HeaderedEvent to indicate if the event is rejected.
	return &respState, nil
}

func (t *missingStateReq) lookupEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, _, missingEventID string, localFirst bool) (*gomatrixserverlib.HeaderedEvent, error) {
	if localFirst {
		// fetch from the roomserver
		queryReq := api.QueryEventsByIDRequest{
			EventIDs: []string{missingEventID},
		}
		var queryRes api.QueryEventsByIDResponse
		if err := t.queryer.QueryEventsByID(ctx, &queryReq, &queryRes); err != nil {
			util.GetLogger(ctx).Warnf("Failed to query roomserver for missing event %s: %s - falling back to remote", missingEventID, err)
		} else if len(queryRes.Events) == 1 {
			return queryRes.Events[0], nil
		}
	}
	var event *gomatrixserverlib.Event
	found := false
	for _, serverName := range t.servers {
		reqctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()
		txn, err := t.federation.GetEvent(reqctx, serverName, missingEventID)
		if err != nil || len(txn.PDUs) == 0 {
			util.GetLogger(ctx).WithError(err).WithField("event_id", missingEventID).Warn("Failed to get missing /event for event ID")
			if errors.Is(err, context.DeadlineExceeded) {
				select {
				case <-reqctx.Done(): // this server took too long
					continue
				case <-ctx.Done(): // the input request timed out
					return nil, context.DeadlineExceeded
				}
			}
			continue
		}
		event, err = gomatrixserverlib.NewEventFromUntrustedJSON(txn.PDUs[0], roomVersion)
		if err != nil {
			util.GetLogger(ctx).WithError(err).WithField("event_id", missingEventID).Warnf("Failed to parse event JSON of event returned from /event")
			continue
		}
		found = true
		break
	}
	if !found {
		util.GetLogger(ctx).WithField("event_id", missingEventID).Warnf("Failed to get missing /event for event ID from %d server(s)", len(t.servers))
		return nil, fmt.Errorf("wasn't able to find event via %d server(s)", len(t.servers))
	}
	if err := event.VerifyEventSignatures(ctx, t.keys); err != nil {
		util.GetLogger(ctx).WithError(err).Warnf("Couldn't validate signature of event %q from /event", event.EventID())
		return nil, verifySigError{event.EventID(), err}
	}
	return t.cacheAndReturn(event.Headered(roomVersion)), nil
}

func checkAllowedByState(e *gomatrixserverlib.Event, stateEvents []*gomatrixserverlib.Event) error {
	authUsingState := gomatrixserverlib.NewAuthEvents(nil)
	for i := range stateEvents {
		err := authUsingState.AddEvent(stateEvents[i])
		if err != nil {
			return err
		}
	}
	return gomatrixserverlib.Allowed(e, &authUsingState)
}

func (t *missingStateReq) hadEvent(eventID string) {
	t.hadEventsMutex.Lock()
	defer t.hadEventsMutex.Unlock()
	t.hadEvents[eventID] = true
}

type verifySigError struct {
	eventID string
	err     error
}
type missingPrevEventsError struct {
	eventID string
	err     error
}

func (e verifySigError) Error() string {
	return fmt.Sprintf("unable to verify signature of event %q: %s", e.eventID, e.err)
}
func (e missingPrevEventsError) Error() string {
	return fmt.Sprintf("unable to get prev_events for event %q: %s", e.eventID, e.err)
}
