// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package eventutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"

	"github.com/matrix-org/gomatrixserverlib"
)

// ErrRoomNoExists is returned when trying to lookup the state of a room that
// doesn't exist
var ErrRoomNoExists = errors.New("room does not exist")

// QueryAndBuildEvent builds a Matrix event using the event builder and roomserver query
// API client provided. If also fills roomserver query API response (if provided)
// in case the function calling FillBuilder needs to use it.
// Returns ErrRoomNoExists if the state of the room could not be retrieved because
// the room doesn't exist
// Returns an error if something else went wrong
func QueryAndBuildEvent(
	ctx context.Context,
	builder *gomatrixserverlib.EventBuilder, cfg *config.Global,
	identity *gomatrixserverlib.SigningIdentity, evTime time.Time,
	rsAPI api.QueryLatestEventsAndStateAPI, queryRes *api.QueryLatestEventsAndStateResponse,
) (*gomatrixserverlib.HeaderedEvent, error) {
	if queryRes == nil {
		queryRes = &api.QueryLatestEventsAndStateResponse{}
	}

	eventsNeeded, err := queryRequiredEventsForBuilder(ctx, builder, rsAPI, queryRes)
	if err != nil {
		// This can pass through a ErrRoomNoExists to the caller
		return nil, err
	}
	return BuildEvent(ctx, builder, cfg, identity, evTime, eventsNeeded, queryRes)
}

// BuildEvent builds a Matrix event from the builder and QueryLatestEventsAndStateResponse
// provided.
func BuildEvent(
	ctx context.Context,
	builder *gomatrixserverlib.EventBuilder, cfg *config.Global,
	identity *gomatrixserverlib.SigningIdentity, evTime time.Time,
	eventsNeeded *gomatrixserverlib.StateNeeded, queryRes *api.QueryLatestEventsAndStateResponse,
) (*gomatrixserverlib.HeaderedEvent, error) {
	if err := addPrevEventsToEvent(builder, eventsNeeded, queryRes); err != nil {
		return nil, err
	}

	event, err := builder.Build(
		evTime, identity.ServerName, identity.KeyID,
		identity.PrivateKey, queryRes.RoomVersion,
	)
	if err != nil {
		return nil, err
	}

	return event.Headered(queryRes.RoomVersion), nil
}

// queryRequiredEventsForBuilder queries the roomserver for auth/prev events needed for this builder.
func queryRequiredEventsForBuilder(
	ctx context.Context,
	builder *gomatrixserverlib.EventBuilder,
	rsAPI api.QueryLatestEventsAndStateAPI, queryRes *api.QueryLatestEventsAndStateResponse,
) (*gomatrixserverlib.StateNeeded, error) {
	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return nil, fmt.Errorf("gomatrixserverlib.StateNeededForEventBuilder: %w", err)
	}

	if len(eventsNeeded.Tuples()) == 0 {
		return nil, errors.New("expecting state tuples for event builder, got none")
	}

	// Ask the roomserver for information about this room
	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID:       builder.RoomID,
		StateToFetch: eventsNeeded.Tuples(),
	}
	return &eventsNeeded, rsAPI.QueryLatestEventsAndState(ctx, &queryReq, queryRes)
}

// addPrevEventsToEvent fills out the prev_events and auth_events fields in builder
func addPrevEventsToEvent(
	builder *gomatrixserverlib.EventBuilder,
	eventsNeeded *gomatrixserverlib.StateNeeded,
	queryRes *api.QueryLatestEventsAndStateResponse,
) error {
	if !queryRes.RoomExists {
		return ErrRoomNoExists
	}

	eventFormat, err := queryRes.RoomVersion.EventFormat()
	if err != nil {
		return fmt.Errorf("queryRes.RoomVersion.EventFormat: %w", err)
	}

	builder.Depth = queryRes.Depth

	authEvents := gomatrixserverlib.NewAuthEvents(nil)

	for i := range queryRes.StateEvents {
		err = authEvents.AddEvent(queryRes.StateEvents[i].Event)
		if err != nil {
			return fmt.Errorf("authEvents.AddEvent: %w", err)
		}
	}

	refs, err := eventsNeeded.AuthEventReferences(&authEvents)
	if err != nil {
		return fmt.Errorf("eventsNeeded.AuthEventReferences: %w", err)
	}

	truncAuth, truncPrev := truncateAuthAndPrevEvents(refs, queryRes.LatestEvents)
	switch eventFormat {
	case gomatrixserverlib.EventFormatV1:
		builder.AuthEvents = truncAuth
		builder.PrevEvents = truncPrev
	case gomatrixserverlib.EventFormatV2:
		v2AuthRefs, v2PrevRefs := []string{}, []string{}
		for _, ref := range truncAuth {
			v2AuthRefs = append(v2AuthRefs, ref.EventID)
		}
		for _, ref := range truncPrev {
			v2PrevRefs = append(v2PrevRefs, ref.EventID)
		}
		builder.AuthEvents = v2AuthRefs
		builder.PrevEvents = v2PrevRefs
	}

	return nil
}

// truncateAuthAndPrevEvents limits the number of events we add into
// an event as prev_events or auth_events.
// NOTSPEC: The limits here feel a bit arbitrary but they are currently
// here because of https://github.com/matrix-org/matrix-doc/issues/2307
// and because Synapse will just drop events that don't comply.
func truncateAuthAndPrevEvents(auth, prev []gomatrixserverlib.EventReference) (
	truncAuth, truncPrev []gomatrixserverlib.EventReference,
) {
	truncAuth, truncPrev = auth, prev
	if len(truncAuth) > 10 {
		truncAuth = truncAuth[:10]
	}
	if len(truncPrev) > 20 {
		truncPrev = truncPrev[:20]
	}
	return
}

// RedactEvent redacts the given event and sets the unsigned field appropriately. This should be used by
// downstream components to the roomserver when an OutputTypeRedactedEvent occurs.
func RedactEvent(redactionEvent, redactedEvent *gomatrixserverlib.Event) error {
	// sanity check
	if redactionEvent.Type() != gomatrixserverlib.MRoomRedaction {
		return fmt.Errorf("RedactEvent: redactionEvent isn't a redaction event, is '%s'", redactionEvent.Type())
	}
	redactedEvent.Redact()
	if err := redactedEvent.SetUnsignedField("redacted_because", redactionEvent); err != nil {
		return err
	}
	// NOTSPEC: sytest relies on this unspecced field existing :(
	if err := redactedEvent.SetUnsignedField("redacted_by", redactionEvent.EventID()); err != nil {
		return err
	}
	return nil
}
