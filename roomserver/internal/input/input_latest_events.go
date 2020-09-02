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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// updateLatestEvents updates the list of latest events for this room in the database and writes the
// event to the output log.
// The latest events are the events that aren't referenced by another event in the database:
//
//     Time goes down the page. 1 is the m.room.create event (root).
//
//        1                 After storing 1 the latest events are {1}
//        |                 After storing 2 the latest events are {2}
//        2                 After storing 3 the latest events are {3}
//       / \                After storing 4 the latest events are {3,4}
//      3   4               After storing 5 the latest events are {5,4}
//      |   |               After storing 6 the latest events are {5,6}
//      5   6 <--- latest   After storing 7 the latest events are {6,7}
//      |
//      7 <----- latest
//
// Can only be called once at a time
func (r *Inputer) updateLatestEvents(
	ctx context.Context,
	roomInfo *types.RoomInfo,
	stateAtEvent types.StateAtEvent,
	event gomatrixserverlib.Event,
	sendAsServer string,
	transactionID *api.TransactionID,
) (err error) {
	updater, err := r.DB.GetLatestEventsForUpdate(ctx, *roomInfo)
	if err != nil {
		return fmt.Errorf("r.DB.GetLatestEventsForUpdate: %w", err)
	}
	succeeded := false
	defer sqlutil.EndTransactionWithCheck(updater, &succeeded, &err)

	u := latestEventsUpdater{
		ctx:           ctx,
		api:           r,
		updater:       updater,
		roomInfo:      roomInfo,
		stateAtEvent:  stateAtEvent,
		event:         event,
		sendAsServer:  sendAsServer,
		transactionID: transactionID,
	}

	if err = u.doUpdateLatestEvents(); err != nil {
		return fmt.Errorf("u.doUpdateLatestEvents: %w", err)
	}

	succeeded = true
	return
}

// latestEventsUpdater tracks the state used to update the latest events in the
// room. It mostly just ferries state between the various function calls.
// The state could be passed using function arguments, but it becomes impractical
// when there are so many variables to pass around.
type latestEventsUpdater struct {
	ctx           context.Context
	api           *Inputer
	updater       *shared.LatestEventsUpdater
	roomInfo      *types.RoomInfo
	stateAtEvent  types.StateAtEvent
	event         gomatrixserverlib.Event
	transactionID *api.TransactionID
	// Which server to send this event as.
	sendAsServer string
	// The eventID of the event that was processed before this one.
	lastEventIDSent string
	// The latest events in the room after processing this event.
	latest []types.StateAtEventAndReference
	// The state entries removed from and added to the current state of the
	// room as a result of processing this event. They are sorted lists.
	removed []types.StateEntry
	added   []types.StateEntry
	// The state entries that are removed and added to recover the state before
	// the event being processed. They are sorted lists.
	stateBeforeEventRemoves []types.StateEntry
	stateBeforeEventAdds    []types.StateEntry
	// The snapshots of current state before and after processing this event
	oldStateNID types.StateSnapshotNID
	newStateNID types.StateSnapshotNID
}

func (u *latestEventsUpdater) doUpdateLatestEvents() error {
	prevEvents := u.event.PrevEvents()
	u.lastEventIDSent = u.updater.LastEventIDSent()
	u.oldStateNID = u.updater.CurrentStateSnapshotNID()

	// If we are doing a regular event update then we will get the
	// previous latest events to use as a part of the calculation. If
	// we are overwriting the latest events because we have a complete
	// state snapshot from somewhere else, e.g. a federated room join,
	// then start with an empty set - none of the forward extremities
	// that we knew about before matter anymore.
	oldLatest := []types.StateAtEventAndReference{}
	if !u.stateAtEvent.Overwrite {
		oldLatest = u.updater.LatestEvents()
	}

	// If the event has already been written to the output log then we
	// don't need to do anything, as we've handled it already.
	hasBeenSent, err := u.updater.HasEventBeenSent(u.stateAtEvent.EventNID)
	if err != nil {
		return fmt.Errorf("u.updater.HasEventBeenSent: %w", err)
	} else if hasBeenSent {
		return nil
	}

	// Update the roomserver_previous_events table with references. This
	// is effectively tracking the structure of the DAG.
	if err = u.updater.StorePreviousEvents(u.stateAtEvent.EventNID, prevEvents); err != nil {
		return fmt.Errorf("u.updater.StorePreviousEvents: %w", err)
	}

	// Get the event reference for our new event. This will be used when
	// determining if the event is referenced by an existing event.
	eventReference := u.event.EventReference()

	// Check if our new event is already referenced by an existing event
	// in the room. If it is then it isn't a latest event.
	alreadyReferenced, err := u.updater.IsReferenced(eventReference)
	if err != nil {
		return fmt.Errorf("u.updater.IsReferenced: %w", err)
	}

	// Work out what the latest events are.
	u.latest = calculateLatest(
		oldLatest,
		alreadyReferenced,
		prevEvents,
		types.StateAtEventAndReference{
			EventReference: eventReference,
			StateAtEvent:   u.stateAtEvent,
		},
	)

	// Now that we know what the latest events are, it's time to get the
	// latest state.
	if err = u.latestState(); err != nil {
		return fmt.Errorf("u.latestState: %w", err)
	}

	// If we need to generate any output events then here's where we do it.
	// TODO: Move this!
	updates, err := u.api.updateMemberships(u.ctx, u.updater, u.removed, u.added)
	if err != nil {
		return fmt.Errorf("u.api.updateMemberships: %w", err)
	}

	update, err := u.makeOutputNewRoomEvent()
	if err != nil {
		return fmt.Errorf("u.makeOutputNewRoomEvent: %w", err)
	}
	updates = append(updates, *update)

	// Send the event to the output logs.
	// We do this inside the database transaction to ensure that we only mark an event as sent if we sent it.
	// (n.b. this means that it's possible that the same event will be sent twice if the transaction fails but
	//  the write to the output log succeeds)
	// TODO: This assumes that writing the event to the output log is synchronous. It should be possible to
	// send the event asynchronously but we would need to ensure that 1) the events are written to the log in
	// the correct order, 2) that pending writes are resent across restarts. In order to avoid writing all the
	// necessary bookkeeping we'll keep the event sending synchronous for now.
	if err = u.api.WriteOutputEvents(u.event.RoomID(), updates); err != nil {
		return fmt.Errorf("u.api.WriteOutputEvents: %w", err)
	}

	if err = u.updater.SetLatestEvents(u.roomInfo.RoomNID, u.latest, u.stateAtEvent.EventNID, u.newStateNID); err != nil {
		return fmt.Errorf("u.updater.SetLatestEvents: %w", err)
	}

	if err = u.updater.MarkEventAsSent(u.stateAtEvent.EventNID); err != nil {
		return fmt.Errorf("u.updater.MarkEventAsSent: %w", err)
	}

	return nil
}

func (u *latestEventsUpdater) latestState() error {
	var err error
	roomState := state.NewStateResolution(u.api.DB, *u.roomInfo)

	// Get a list of the current latest events.
	latestStateAtEvents := make([]types.StateAtEvent, len(u.latest))
	for i := range u.latest {
		latestStateAtEvents[i] = u.latest[i].StateAtEvent
	}

	// Takes the NIDs of the latest events and creates a state snapshot
	// of the state after the events. The snapshot state will be resolved
	// using the correct state resolution algorithm for the room.
	u.newStateNID, err = roomState.CalculateAndStoreStateAfterEvents(
		u.ctx, latestStateAtEvents,
	)
	if err != nil {
		return fmt.Errorf("roomState.CalculateAndStoreStateAfterEvents: %w", err)
	}

	// If we are overwriting the state then we should make sure that we
	// don't send anything out over federation again, it will very likely
	// be a repeat.
	if u.stateAtEvent.Overwrite {
		u.sendAsServer = ""
	}

	// Now that we have a new state snapshot based on the latest events,
	// we can compare that new snapshot to the previous one and see what
	// has changed. This gives us one list of removed state events and
	// another list of added ones. Replacing a value for a state-key tuple
	// will result one removed (the old event) and one added (the new event).
	u.removed, u.added, err = roomState.DifferenceBetweeenStateSnapshots(
		u.ctx, u.oldStateNID, u.newStateNID,
	)
	if err != nil {
		return fmt.Errorf("roomState.DifferenceBetweenStateSnapshots: %w", err)
	}

	// Also work out the state before the event removes and the event
	// adds.
	u.stateBeforeEventRemoves, u.stateBeforeEventAdds, err = roomState.DifferenceBetweeenStateSnapshots(
		u.ctx, u.newStateNID, u.stateAtEvent.BeforeStateSnapshotNID,
	)
	if err != nil {
		return fmt.Errorf("roomState.DifferenceBetweeenStateSnapshots: %w", err)
	}

	return nil
}

func calculateLatest(
	oldLatest []types.StateAtEventAndReference,
	alreadyReferenced bool,
	prevEvents []gomatrixserverlib.EventReference,
	newEvent types.StateAtEventAndReference,
) []types.StateAtEventAndReference {
	var alreadyInLatest bool
	var newLatest []types.StateAtEventAndReference
	for _, l := range oldLatest {
		keep := true
		for _, prevEvent := range prevEvents {
			if l.EventID == prevEvent.EventID && bytes.Equal(l.EventSHA256, prevEvent.EventSHA256) {
				// This event can be removed from the latest events cause we've found an event that references it.
				// (If an event is referenced by another event then it can't be one of the latest events in the room
				//  because we have an event that comes after it)
				keep = false
				break
			}
		}
		if l.EventNID == newEvent.EventNID {
			alreadyInLatest = true
		}
		if keep {
			// Keep the event in the latest events.
			newLatest = append(newLatest, l)
		}
	}

	if !alreadyReferenced && !alreadyInLatest {
		// This event is not referenced by any of the events in the room
		// and the event is not already in the latest events.
		// Add it to the latest events
		newLatest = append(newLatest, newEvent)
	}

	return newLatest
}

func (u *latestEventsUpdater) makeOutputNewRoomEvent() (*api.OutputEvent, error) {

	latestEventIDs := make([]string, len(u.latest))
	for i := range u.latest {
		latestEventIDs[i] = u.latest[i].EventID
	}

	ore := api.OutputNewRoomEvent{
		Event:           u.event.Headered(u.roomInfo.RoomVersion),
		LastSentEventID: u.lastEventIDSent,
		LatestEventIDs:  latestEventIDs,
		TransactionID:   u.transactionID,
	}

	eventIDMap, err := u.stateEventMap()
	if err != nil {
		return nil, err
	}

	for _, entry := range u.added {
		ore.AddsStateEventIDs = append(ore.AddsStateEventIDs, eventIDMap[entry.EventNID])
	}
	for _, entry := range u.removed {
		ore.RemovesStateEventIDs = append(ore.RemovesStateEventIDs, eventIDMap[entry.EventNID])
	}
	for _, entry := range u.stateBeforeEventRemoves {
		ore.StateBeforeRemovesEventIDs = append(ore.StateBeforeRemovesEventIDs, eventIDMap[entry.EventNID])
	}
	for _, entry := range u.stateBeforeEventAdds {
		ore.StateBeforeAddsEventIDs = append(ore.StateBeforeAddsEventIDs, eventIDMap[entry.EventNID])
	}
	ore.SendAsServer = u.sendAsServer

	// include extra state events if they were added as nearly every downstream component will care about it
	// and we'd rather not have them all hit QueryEventsByID at the same time!
	if len(ore.AddsStateEventIDs) > 0 {
		ore.AddStateEvents, err = u.extraEventsForIDs(u.roomInfo.RoomVersion, ore.AddsStateEventIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to load add_state_events from db: %w", err)
		}
	}

	return &api.OutputEvent{
		Type:         api.OutputTypeNewRoomEvent,
		NewRoomEvent: &ore,
	}, nil
}

// extraEventsForIDs returns the full events for the event IDs given, but does not include the current event being
// updated.
func (u *latestEventsUpdater) extraEventsForIDs(roomVersion gomatrixserverlib.RoomVersion, eventIDs []string) ([]gomatrixserverlib.HeaderedEvent, error) {
	var extraEventIDs []string
	for _, e := range eventIDs {
		if e == u.event.EventID() {
			continue
		}
		extraEventIDs = append(extraEventIDs, e)
	}
	if len(extraEventIDs) == 0 {
		return nil, nil
	}
	extraEvents, err := u.api.DB.EventsFromIDs(u.ctx, extraEventIDs)
	if err != nil {
		return nil, err
	}
	var h []gomatrixserverlib.HeaderedEvent
	for _, e := range extraEvents {
		h = append(h, e.Headered(roomVersion))
	}
	return h, nil
}

// retrieve an event nid -> event ID map for all events that need updating
func (u *latestEventsUpdater) stateEventMap() (map[types.EventNID]string, error) {
	var stateEventNIDs []types.EventNID
	var allStateEntries []types.StateEntry
	allStateEntries = append(allStateEntries, u.added...)
	allStateEntries = append(allStateEntries, u.removed...)
	allStateEntries = append(allStateEntries, u.stateBeforeEventRemoves...)
	allStateEntries = append(allStateEntries, u.stateBeforeEventAdds...)
	for _, entry := range allStateEntries {
		stateEventNIDs = append(stateEventNIDs, entry.EventNID)
	}
	stateEventNIDs = stateEventNIDs[:util.SortAndUnique(eventNIDSorter(stateEventNIDs))]
	return u.api.DB.EventIDs(u.ctx, stateEventNIDs)
}

type eventNIDSorter []types.EventNID

func (s eventNIDSorter) Len() int           { return len(s) }
func (s eventNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s eventNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
