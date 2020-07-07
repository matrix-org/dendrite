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

package internal

import (
	"context"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// processRoomEvent can only be called once at a time
//
// TODO(#375): This should be rewritten to allow concurrent calls. The
// difficulty is in ensuring that we correctly annotate events with the correct
// state deltas when sending to kafka streams
// TODO: Break up function - we should probably do transaction ID checks before calling this.
// nolint:gocyclo
func (r *RoomserverInternalAPI) processRoomEvent(
	ctx context.Context,
	input api.InputRoomEvent,
) (eventID string, err error) {
	// Parse and validate the event JSON
	headered := input.Event
	event := headered.Unwrap()

	// Check that the event passes authentication checks and work out
	// the numeric IDs for the auth events.
	authEventNIDs, err := checkAuthEvents(ctx, r.DB, headered, input.AuthEventIDs)
	if err != nil {
		logrus.WithError(err).WithField("event_id", event.EventID()).WithField("auth_event_ids", input.AuthEventIDs).Error("processRoomEvent.checkAuthEvents failed for event")
		return
	}

	// If we don't have a transaction ID then get one.
	if input.TransactionID != nil {
		tdID := input.TransactionID
		eventID, err = r.DB.GetTransactionEventID(
			ctx, tdID.TransactionID, tdID.SessionID, event.Sender(),
		)
		// On error OR event with the transaction already processed/processesing
		if err != nil || eventID != "" {
			return
		}
	}

	// Store the event.
	roomNID, stateAtEvent, redactionEvent, redactedEventID, err := r.DB.StoreEvent(ctx, event, input.TransactionID, authEventNIDs)
	if err != nil {
		return
	}
	// if storing this event results in it being redacted then do so.
	if redactedEventID == event.EventID() {
		r, rerr := eventutil.RedactEvent(redactionEvent, &event)
		if rerr != nil {
			return "", rerr
		}
		event = *r
	}

	// For outliers we can stop after we've stored the event itself as it
	// doesn't have any associated state to store and we don't need to
	// notify anyone about it.
	if input.Kind == api.KindOutlier {
		logrus.WithFields(logrus.Fields{
			"event_id": event.EventID(),
			"type":     event.Type(),
			"room":     event.RoomID(),
		}).Info("Stored outlier")
		return event.EventID(), nil
	}

	if stateAtEvent.BeforeStateSnapshotNID == 0 {
		// We haven't calculated a state for this event yet.
		// Lets calculate one.
		err = r.calculateAndSetState(ctx, input, roomNID, &stateAtEvent, event)
		if err != nil {
			return
		}
	}

	if err = r.updateLatestEvents(
		ctx,                 // context
		roomNID,             // room NID to update
		stateAtEvent,        // state at event (below)
		event,               // event
		input.SendAsServer,  // send as server
		input.TransactionID, // transaction ID
	); err != nil {
		return
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
			return
		}
	}

	// Update the extremities of the event graph for the room
	return event.EventID(), nil
}

func (r *RoomserverInternalAPI) calculateAndSetState(
	ctx context.Context,
	input api.InputRoomEvent,
	roomNID types.RoomNID,
	stateAtEvent *types.StateAtEvent,
	event gomatrixserverlib.Event,
) error {
	var err error
	roomState := state.NewStateResolution(r.DB)

	if input.HasState {
		// Check here if we think we're in the room already.
		stateAtEvent.Overwrite = true
		var joinEventNIDs []types.EventNID
		// Request join memberships only for local users only.
		if joinEventNIDs, err = r.DB.GetMembershipEventNIDsForRoom(ctx, roomNID, true, true); err == nil {
			// If we have no local users that are joined to the room then any state about
			// the room that we have is quite possibly out of date. Therefore in that case
			// we should overwrite it rather than merge it.
			stateAtEvent.Overwrite = len(joinEventNIDs) == 0
		}

		// We've been told what the state at the event is so we don't need to calculate it.
		// Check that those state events are in the database and store the state.
		var entries []types.StateEntry
		if entries, err = r.DB.StateEntriesForEventIDs(ctx, input.StateEventIDs); err != nil {
			return err
		}

		if stateAtEvent.BeforeStateSnapshotNID, err = r.DB.AddState(ctx, roomNID, nil, entries); err != nil {
			return err
		}
	} else {
		stateAtEvent.Overwrite = false

		// We haven't been told what the state at the event is so we need to calculate it from the prev_events
		if stateAtEvent.BeforeStateSnapshotNID, err = roomState.CalculateAndStoreStateBeforeEvent(ctx, event, roomNID); err != nil {
			return err
		}
	}
	return r.DB.SetState(ctx, stateAtEvent.EventNID, stateAtEvent.BeforeStateSnapshotNID)
}
