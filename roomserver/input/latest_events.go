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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
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
func updateLatestEvents(
	ctx context.Context,
	db RoomEventDatabase,
	ow OutputRoomEventWriter,
	roomNID types.RoomNID,
	stateAtEvent types.StateAtEvent,
	event gomatrixserverlib.Event,
	sendAsServer string,
	transactionID *api.TransactionID,
) (err error) {
	updater, err := db.GetLatestEventsForUpdate(ctx, roomNID)
	if err != nil {
		return
	}
	succeeded := false
	defer func() {
		txerr := common.EndTransaction(updater, &succeeded)
		if err == nil && txerr != nil {
			err = txerr
		}
	}()

	roomVersion, err := db.GetRoomVersionForRoom(ctx, roomNID)
	if err != nil {
		return err
	}

	u := latestEventsUpdater{
		ctx: ctx, db: db, updater: updater, ow: ow, roomNID: roomNID,
		roomVersion: roomVersion, stateAtEvent: stateAtEvent, event: event,
		sendAsServer: sendAsServer, transactionID: transactionID,
	}
	if err = u.doUpdateLatestEvents(); err != nil {
		return err
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
	db            RoomEventDatabase
	updater       types.RoomRecentEventsUpdater
	ow            OutputRoomEventWriter
	roomNID       types.RoomNID
	roomVersion   gomatrixserverlib.RoomVersion
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
	oldLatest := u.updater.LatestEvents()
	u.lastEventIDSent = u.updater.LastEventIDSent()
	u.oldStateNID = u.updater.CurrentStateSnapshotNID()

	hasBeenSent, err := u.updater.HasEventBeenSent(u.stateAtEvent.EventNID)
	if err != nil {
		return err
	} else if hasBeenSent {
		// Already sent this event so we can stop processing
		return nil
	}

	if err = u.updater.StorePreviousEvents(u.stateAtEvent.EventNID, prevEvents); err != nil {
		return err
	}

	eventReference := u.event.EventReference()
	// Check if this event is already referenced by another event in the room.
	alreadyReferenced, err := u.updater.IsReferenced(eventReference)
	if err != nil {
		return err
	}

	u.latest = calculateLatest(oldLatest, alreadyReferenced, prevEvents, types.StateAtEventAndReference{
		EventReference: eventReference,
		StateAtEvent:   u.stateAtEvent,
	})

	if err = u.latestState(); err != nil {
		return err
	}

	updates, err := updateMemberships(u.ctx, u.db, u.updater, u.removed, u.added)
	if err != nil {
		return err
	}

	update, err := u.makeOutputNewRoomEvent()
	if err != nil {
		return err
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
	if err = u.ow.WriteOutputEvents(u.event.RoomID(), updates); err != nil {
		return err
	}

	if err = u.updater.SetLatestEvents(u.roomNID, u.latest, u.stateAtEvent.EventNID, u.newStateNID); err != nil {
		return err
	}

	return u.updater.MarkEventAsSent(u.stateAtEvent.EventNID)
}

func (u *latestEventsUpdater) latestState() error {
	var err error
	roomVersion, err := u.db.GetRoomVersionForRoom(u.ctx, u.roomNID)
	if err != nil {
		return err
	}
	roomState, err := state.GetStateResolutionAlgorithm(roomVersion, u.db)
	if err != nil {
		return err
	}

	latestStateAtEvents := make([]types.StateAtEvent, len(u.latest))
	for i := range u.latest {
		latestStateAtEvents[i] = u.latest[i].StateAtEvent
	}
	u.newStateNID, err = roomState.CalculateAndStoreStateAfterEvents(
		u.ctx, u.roomNID, latestStateAtEvents,
	)
	if err != nil {
		return err
	}

	u.removed, u.added, err = roomState.DifferenceBetweeenStateSnapshots(
		u.ctx, u.oldStateNID, u.newStateNID,
	)
	if err != nil {
		return err
	}

	u.stateBeforeEventRemoves, u.stateBeforeEventAdds, err = roomState.DifferenceBetweeenStateSnapshots(
		u.ctx, u.newStateNID, u.stateAtEvent.BeforeStateSnapshotNID,
	)
	return err
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

	roomVersion, err := u.db.GetRoomVersionForRoom(context.Background(), u.roomNID)
	if err != nil {
		return nil, err
	}

	ore := api.OutputNewRoomEvent{
		Event:           u.event,
		RoomVersion:     roomVersion,
		LastSentEventID: u.lastEventIDSent,
		LatestEventIDs:  latestEventIDs,
		TransactionID:   u.transactionID,
	}

	var stateEventNIDs []types.EventNID
	for _, entry := range u.added {
		stateEventNIDs = append(stateEventNIDs, entry.EventNID)
	}
	for _, entry := range u.removed {
		stateEventNIDs = append(stateEventNIDs, entry.EventNID)
	}
	for _, entry := range u.stateBeforeEventRemoves {
		stateEventNIDs = append(stateEventNIDs, entry.EventNID)
	}
	for _, entry := range u.stateBeforeEventAdds {
		stateEventNIDs = append(stateEventNIDs, entry.EventNID)
	}
	stateEventNIDs = stateEventNIDs[:util.SortAndUnique(eventNIDSorter(stateEventNIDs))]
	eventIDMap, err := u.db.EventIDs(u.ctx, stateEventNIDs)
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

	return &api.OutputEvent{
		Type:         api.OutputTypeNewRoomEvent,
		NewRoomEvent: &ore,
	}, nil
}

type eventNIDSorter []types.EventNID

func (s eventNIDSorter) Len() int           { return len(s) }
func (s eventNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s eventNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
