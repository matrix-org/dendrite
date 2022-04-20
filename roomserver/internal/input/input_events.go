// Copyright 2017 Vector Creations Ltd
// Copyright 2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package input

import (
	"bytes"
	"context"
	"fmt"
	"time"

	fedapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// TODO: Does this value make sense?
const MaximumMissingProcessingTime = time.Minute * 2

var processRoomEventDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "dendrite",
		Subsystem: "roomserver",
		Name:      "processroomevent_duration_millis",
		Help:      "How long it takes the roomserver to process an event",
		Buckets: []float64{ // milliseconds
			5, 10, 25, 50, 75, 100, 250, 500,
			1000, 2000, 3000, 4000, 5000, 6000,
			7000, 8000, 9000, 10000, 15000, 20000,
		},
	},
	[]string{"room_id"},
)

// processRoomEvent can only be called once at a time
//
// TODO(#375): This should be rewritten to allow concurrent calls. The
// difficulty is in ensuring that we correctly annotate events with the correct
// state deltas when sending to kafka streams
// TODO: Break up function - we should probably do transaction ID checks before calling this.
// nolint:gocyclo
func (r *Inputer) processRoomEvent(
	ctx context.Context,
	input *api.InputRoomEvent,
) error {
	select {
	case <-ctx.Done():
		// Before we do anything, make sure the context hasn't expired for this pending task.
		// If it has then we'll give up straight away — it's probably a synchronous input
		// request and the caller has already given up, but the inbox task was still queued.
		return context.DeadlineExceeded
	default:
	}

	// Measure how long it takes to process this event.
	started := time.Now()
	defer func() {
		timetaken := time.Since(started)
		processRoomEventDuration.With(prometheus.Labels{
			"room_id": input.Event.RoomID(),
		}).Observe(float64(timetaken.Milliseconds()))
	}()

	// Parse and validate the event JSON
	headered := input.Event
	event := headered.Unwrap()
	logger := util.GetLogger(ctx).WithFields(logrus.Fields{
		"event_id": event.EventID(),
		"room_id":  event.RoomID(),
		"kind":     input.Kind,
		"origin":   input.Origin,
		"type":     event.Type(),
	})
	if input.HasState {
		logger = logger.WithFields(logrus.Fields{
			"has_state": input.HasState,
			"state_ids": len(input.StateEventIDs),
		})
	}

	// if we have already got this event then do not process it again, if the input kind is an outlier.
	// Outliers contain no extra information which may warrant a re-processing.
	if input.Kind == api.KindOutlier {
		evs, err2 := r.DB.EventsFromIDs(ctx, []string{event.EventID()})
		if err2 == nil && len(evs) == 1 {
			// check hash matches if we're on early room versions where the event ID was a random string
			idFormat, err2 := headered.RoomVersion.EventIDFormat()
			if err2 == nil {
				switch idFormat {
				case gomatrixserverlib.EventIDFormatV1:
					if bytes.Equal(event.EventReference().EventSHA256, evs[0].EventReference().EventSHA256) {
						logger.Debugf("Already processed event; ignoring")
						return nil
					}
				default:
					logger.Debugf("Already processed event; ignoring")
					return nil
				}
			}
		}
	}

	// Don't waste time processing the event if the room doesn't exist.
	// A room entry locally will only be created in response to a create
	// event.
	roomInfo, rerr := r.DB.RoomInfo(ctx, event.RoomID())
	if rerr != nil {
		return fmt.Errorf("r.DB.RoomInfo: %w", rerr)
	}
	isCreateEvent := event.Type() == gomatrixserverlib.MRoomCreate && event.StateKeyEquals("")
	if roomInfo == nil && !isCreateEvent {
		return fmt.Errorf("room %s does not exist for event %s", event.RoomID(), event.EventID())
	}

	var missingAuth, missingPrev bool
	serverRes := &fedapi.QueryJoinedHostServerNamesInRoomResponse{}
	if !isCreateEvent {
		missingAuthIDs, missingPrevIDs, err := r.DB.MissingAuthPrevEvents(ctx, event)
		if err != nil {
			return fmt.Errorf("updater.MissingAuthPrevEvents: %w", err)
		}
		missingAuth = len(missingAuthIDs) > 0
		missingPrev = !input.HasState && len(missingPrevIDs) > 0
	}

	if missingAuth || missingPrev {
		serverReq := &fedapi.QueryJoinedHostServerNamesInRoomRequest{
			RoomID:      event.RoomID(),
			ExcludeSelf: true,
		}
		if err := r.FSAPI.QueryJoinedHostServerNamesInRoom(ctx, serverReq, serverRes); err != nil {
			return fmt.Errorf("r.FSAPI.QueryJoinedHostServerNamesInRoom: %w", err)
		}
		// Sort all of the servers into a map so that we can randomise
		// their order. Then make sure that the input origin and the
		// event origin are first on the list.
		servers := map[gomatrixserverlib.ServerName]struct{}{}
		for _, server := range serverRes.ServerNames {
			servers[server] = struct{}{}
		}
		serverRes.ServerNames = serverRes.ServerNames[:0]
		if input.Origin != "" {
			serverRes.ServerNames = append(serverRes.ServerNames, input.Origin)
			delete(servers, input.Origin)
		}
		if origin := event.Origin(); origin != input.Origin {
			serverRes.ServerNames = append(serverRes.ServerNames, origin)
			delete(servers, origin)
		}
		for server := range servers {
			serverRes.ServerNames = append(serverRes.ServerNames, server)
			delete(servers, server)
		}
	}

	// First of all, check that the auth events of the event are known.
	// If they aren't then we will ask the federation API for them.
	isRejected := false
	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	knownEvents := map[string]*types.Event{}
	if err := r.fetchAuthEvents(ctx, logger, headered, &authEvents, knownEvents, serverRes.ServerNames); err != nil {
		return fmt.Errorf("r.fetchAuthEvents: %w", err)
	}

	// Check if the event is allowed by its auth events. If it isn't then
	// we consider the event to be "rejected" — it will still be persisted.
	var rejectionErr error
	if rejectionErr = gomatrixserverlib.Allowed(event, &authEvents); rejectionErr != nil {
		isRejected = true
		logger.WithError(rejectionErr).Warnf("Event %s not allowed by auth events", event.EventID())
	}

	// Accumulate the auth event NIDs.
	authEventIDs := event.AuthEventIDs()
	authEventNIDs := make([]types.EventNID, 0, len(authEventIDs))
	for _, authEventID := range authEventIDs {
		if _, ok := knownEvents[authEventID]; !ok {
			// Unknown auth events only really matter if the event actually failed
			// auth. If it passed auth then we can assume that everything that was
			// known was sufficient, even if extraneous auth events were specified
			// but weren't found.
			if isRejected {
				if event.StateKey() != nil {
					return fmt.Errorf(
						"missing auth event %s for state event %s (type %q, state key %q)",
						authEventID, event.EventID(), event.Type(), *event.StateKey(),
					)
				} else {
					return fmt.Errorf(
						"missing auth event %s for timeline event %s (type %q)",
						authEventID, event.EventID(), event.Type(),
					)
				}
			}
		} else {
			authEventNIDs = append(authEventNIDs, knownEvents[authEventID].EventNID)
		}
	}

	var softfail bool
	if input.Kind == api.KindNew {
		// Check that the event passes authentication checks based on the
		// current room state.
		var err error
		softfail, err = helpers.CheckForSoftFail(ctx, r.DB, headered, input.StateEventIDs)
		if err != nil {
			logger.WithError(err).Warn("Error authing soft-failed event")
		}
	}

	// At this point we are checking whether we know all of the prev events, and
	// if we know the state before the prev events. This is necessary before we
	// try to do `calculateAndSetState` on the event later, otherwise it will fail
	// with missing event NIDs. If there's anything missing then we'll go and fetch
	// the prev events and state from the federation. Note that we only do this if
	// we weren't already told what the state before the event should be — if the
	// HasState option was set and a state set was provided (as is the case in a
	// typical federated room join) then we won't bother trying to fetch prev events
	// because we may not be allowed to see them and we have no choice but to trust
	// the state event IDs provided to us in the join instead.
	if missingPrev && input.Kind == api.KindNew {
		// Don't do this for KindOld events, otherwise old events that we fetch
		// to satisfy missing prev events/state will end up recursively calling
		// processRoomEvent.
		if len(serverRes.ServerNames) > 0 {
			missingState := missingStateReq{
				origin:     input.Origin,
				inputer:    r,
				db:         r.DB,
				roomInfo:   roomInfo,
				federation: r.FSAPI,
				keys:       r.KeyRing,
				roomsMu:    internal.NewMutexByRoom(),
				servers:    serverRes.ServerNames,
				hadEvents:  map[string]bool{},
				haveEvents: map[string]*gomatrixserverlib.Event{},
			}
			if stateSnapshot, err := missingState.processEventWithMissingState(ctx, event, headered.RoomVersion); err != nil {
				// Something went wrong with retrieving the missing state, so we can't
				// really do anything with the event other than reject it at this point.
				isRejected = true
				rejectionErr = fmt.Errorf("missingState.processEventWithMissingState: %w", err)
			} else if stateSnapshot != nil {
				// We retrieved some state and we ended up having to call /state_ids for
				// the new event in question (probably because closing the gap by using
				// /get_missing_events didn't do what we hoped) so we'll instead overwrite
				// the state snapshot with the newly resolved state.
				missingPrev = false
				input.HasState = true
				input.StateEventIDs = make([]string, 0, len(stateSnapshot.StateEvents))
				for _, e := range stateSnapshot.StateEvents {
					input.StateEventIDs = append(input.StateEventIDs, e.EventID())
				}
			} else {
				// We retrieved some state and it would appear that rolling forward the
				// state did everything we needed it to do, so we can just resolve the
				// state for the event in the normal way.
				missingPrev = false
			}
		} else {
			// We're missing prev events or state for the event, but for some reason
			// we don't know any servers to ask. In this case we can't do anything but
			// reject the event and hope that it gets unrejected later.
			isRejected = true
			rejectionErr = fmt.Errorf("missing prev events and no other servers to ask")
		}
	}

	// Store the event.
	_, _, stateAtEvent, redactionEvent, redactedEventID, err := r.DB.StoreEvent(ctx, event, authEventNIDs, isRejected || softfail)
	if err != nil {
		return fmt.Errorf("updater.StoreEvent: %w", err)
	}

	// if storing this event results in it being redacted then do so.
	if !isRejected && redactedEventID == event.EventID() {
		r, rerr := eventutil.RedactEvent(redactionEvent, event)
		if rerr != nil {
			return fmt.Errorf("eventutil.RedactEvent: %w", rerr)
		}
		event = r
	}

	// For outliers we can stop after we've stored the event itself as it
	// doesn't have any associated state to store and we don't need to
	// notify anyone about it.
	if input.Kind == api.KindOutlier {
		logger.Debug("Stored outlier")
		hooks.Run(hooks.KindNewEventPersisted, headered)
		return nil
	}

	// Request the room info again — it's possible that the room has been
	// created by now if it didn't exist already.
	roomInfo, err = r.DB.RoomInfo(ctx, event.RoomID())
	if err != nil {
		return fmt.Errorf("updater.RoomInfo: %w", err)
	}
	if roomInfo == nil {
		return fmt.Errorf("updater.RoomInfo missing for room %s", event.RoomID())
	}

	if input.HasState || (!missingPrev && stateAtEvent.BeforeStateSnapshotNID == 0) {
		// We haven't calculated a state for this event yet.
		// Lets calculate one.
		err = r.calculateAndSetState(ctx, input, roomInfo, &stateAtEvent, event, isRejected)
		if err != nil {
			return fmt.Errorf("r.calculateAndSetState: %w", err)
		}
	}

	// We stop here if the event is rejected: We've stored it but won't update forward extremities or notify anyone about it.
	if isRejected || softfail {
		logger.WithError(rejectionErr).WithFields(logrus.Fields{
			"soft_fail":    softfail,
			"missing_prev": missingPrev,
		}).Warn("Stored rejected event")
		if rejectionErr != nil {
			return types.RejectedError(rejectionErr.Error())
		}
		return nil
	}

	switch input.Kind {
	case api.KindNew:
		if err = r.updateLatestEvents(
			ctx,                 // context
			roomInfo,            // room info for the room being updated
			stateAtEvent,        // state at event (below)
			event,               // event
			input.SendAsServer,  // send as server
			input.TransactionID, // transaction ID
			input.HasState,      // rewrites state?
		); err != nil {
			return fmt.Errorf("r.updateLatestEvents: %w", err)
		}
	case api.KindOld:
		err = r.WriteOutputEvents(event.RoomID(), []api.OutputEvent{
			{
				Type: api.OutputTypeOldRoomEvent,
				OldRoomEvent: &api.OutputOldRoomEvent{
					Event: headered,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("r.WriteOutputEvents (old): %w", err)
		}
	}

	// processing this event resulted in an event (which may not be the one we're processing)
	// being redacted. We are guaranteed to have both sides (the redaction/redacted event),
	// so notify downstream components to redact this event - they should have it if they've
	// been tracking our output log.
	if redactedEventID != "" {
		err = r.WriteOutputEvents(event.RoomID(), []api.OutputEvent{
			{
				Type: api.OutputTypeRedactedEvent,
				RedactedEvent: &api.OutputRedactedEvent{
					RedactedEventID: redactedEventID,
					RedactedBecause: redactionEvent.Headered(headered.RoomVersion),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("r.WriteOutputEvents (redactions): %w", err)
		}
	}

	// Everything was OK — the latest events updater didn't error and
	// we've sent output events. Finally, generate a hook call.
	hooks.Run(hooks.KindNewEventPersisted, headered)
	return nil
}

// fetchAuthEvents will check to see if any of the
// auth events specified by the given event are unknown. If they are
// then we will go off and request them from the federation and then
// store them in the database. By the time this function ends, either
// we've failed to retrieve the auth chain altogether (in which case
// an error is returned) or we've successfully retrieved them all and
// they are now in the database.
func (r *Inputer) fetchAuthEvents(
	ctx context.Context,
	logger *logrus.Entry,
	event *gomatrixserverlib.HeaderedEvent,
	auth *gomatrixserverlib.AuthEvents,
	known map[string]*types.Event,
	servers []gomatrixserverlib.ServerName,
) error {
	unknown := map[string]struct{}{}
	authEventIDs := event.AuthEventIDs()
	if len(authEventIDs) == 0 {
		return nil
	}

	for _, authEventID := range authEventIDs {
		authEvents, err := r.DB.EventsFromIDs(ctx, []string{authEventID})
		if err != nil || len(authEvents) == 0 || authEvents[0].Event == nil {
			unknown[authEventID] = struct{}{}
			continue
		}
		ev := authEvents[0]
		known[authEventID] = &ev // don't take the pointer of the iterated event
		if err = auth.AddEvent(ev.Event); err != nil {
			return fmt.Errorf("auth.AddEvent: %w", err)
		}
	}

	// If there are no missing auth events then there is nothing more
	// to do — we've loaded everything that we need.
	if len(unknown) == 0 {
		return nil
	}

	var err error
	var res gomatrixserverlib.RespEventAuth
	var found bool
	for _, serverName := range servers {
		// Request the entire auth chain for the event in question. This should
		// contain all of the auth events — including ones that we already know —
		// so we'll need to filter through those in the next section.
		res, err = r.FSAPI.GetEventAuth(ctx, serverName, event.RoomVersion, event.RoomID(), event.EventID())
		if err != nil {
			logger.WithError(err).Warnf("Failed to get event auth from federation for %q: %s", event.EventID(), err)
			continue
		}
		found = true
		break
	}
	if !found {
		return fmt.Errorf("no servers provided event auth for event ID %q, tried servers %v", event.EventID(), servers)
	}

	// Reuse these to reduce allocations.
	authEventNIDs := make([]types.EventNID, 0, 5)
	isRejected := false
nextAuthEvent:
	for _, authEvent := range gomatrixserverlib.ReverseTopologicalOrdering(
		res.AuthEvents.UntrustedEvents(event.RoomVersion),
		gomatrixserverlib.TopologicalOrderByAuthEvents,
	) {
		// If we already know about this event from the database then we don't
		// need to store it again or do anything further with it, so just skip
		// over it rather than wasting cycles.
		if ev, ok := known[authEvent.EventID()]; ok && ev != nil {
			continue nextAuthEvent
		}

		// Check the signatures of the event. If this fails then we'll simply
		// skip it, because gomatrixserverlib.Allowed() will notice a problem
		// if a critical event is missing anyway.
		if err := authEvent.VerifyEventSignatures(ctx, r.FSAPI.KeyRing()); err != nil {
			continue nextAuthEvent
		}

		// In order to store the new auth event, we need to know its auth chain
		// as NIDs for the `auth_event_nids` column. Let's see if we can find those.
		authEventNIDs = authEventNIDs[:0]
		for _, eventID := range authEvent.AuthEventIDs() {
			knownEvent, ok := known[eventID]
			if !ok {
				continue nextAuthEvent
			}
			authEventNIDs = append(authEventNIDs, knownEvent.EventNID)
		}

		// Check if the auth event should be rejected.
		err := gomatrixserverlib.Allowed(authEvent, auth)
		if isRejected = err != nil; isRejected {
			logger.WithError(err).Warnf("Auth event %s rejected", authEvent.EventID())
		}

		// Finally, store the event in the database.
		eventNID, _, _, _, _, err := r.DB.StoreEvent(ctx, authEvent, authEventNIDs, isRejected)
		if err != nil {
			return fmt.Errorf("updater.StoreEvent: %w", err)
		}

		// Let's take a note of the fact that we now know about this event for
		// authenticating future events.
		if !isRejected {
			if err := auth.AddEvent(authEvent); err != nil {
				return fmt.Errorf("auth.AddEvent: %w", err)
			}
		}

		// Now we know about this event, it was stored and the signatures were OK.
		known[authEvent.EventID()] = &types.Event{
			EventNID: eventNID,
			Event:    authEvent,
		}
	}

	return nil
}

func (r *Inputer) calculateAndSetState(
	ctx context.Context,
	input *api.InputRoomEvent,
	roomInfo *types.RoomInfo,
	stateAtEvent *types.StateAtEvent,
	event *gomatrixserverlib.Event,
	isRejected bool,
) error {
	var succeeded bool
	updater, err := r.DB.GetRoomUpdater(ctx, roomInfo)
	if err != nil {
		return fmt.Errorf("r.DB.GetRoomUpdater: %w", err)
	}
	defer sqlutil.EndTransactionWithCheck(updater, &succeeded, &err)
	roomState := state.NewStateResolution(updater, roomInfo)

	if input.HasState {
		stateAtEvent.Overwrite = true

		// We've been told what the state at the event is so we don't need to calculate it.
		// Check that those state events are in the database and store the state.
		var entries []types.StateEntry
		if entries, err = r.DB.StateEntriesForEventIDs(ctx, input.StateEventIDs); err != nil {
			return fmt.Errorf("updater.StateEntriesForEventIDs: %w", err)
		}
		entries = types.DeduplicateStateEntries(entries)

		if stateAtEvent.BeforeStateSnapshotNID, err = updater.AddState(ctx, roomInfo.RoomNID, nil, entries); err != nil {
			return fmt.Errorf("updater.AddState: %w", err)
		}
	} else {
		stateAtEvent.Overwrite = false

		// We haven't been told what the state at the event is so we need to calculate it from the prev_events
		if stateAtEvent.BeforeStateSnapshotNID, err = roomState.CalculateAndStoreStateBeforeEvent(ctx, event, isRejected); err != nil {
			return fmt.Errorf("roomState.CalculateAndStoreStateBeforeEvent: %w", err)
		}
	}

	err = updater.SetState(ctx, stateAtEvent.EventNID, stateAtEvent.BeforeStateSnapshotNID)
	if err != nil {
		return fmt.Errorf("r.DB.SetState: %w", err)
	}
	succeeded = true
	return nil
}
