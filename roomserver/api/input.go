// Copyright 2017 Vector Creations Ltd
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

// Package api provides the types that are used to communicate with the roomserver.
package api

import (
	"context"
	"net/http"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	"github.com/matrix-org/gomatrixserverlib"
	opentracing "github.com/opentracing/opentracing-go"
)

const (
	// KindOutlier event fall outside the contiguous event graph.
	// We do not have the state for these events.
	// These events are state events used to authenticate other events.
	// They can become part of the contiguous event graph via backfill.
	KindOutlier = 1
	// KindNew event extend the contiguous graph going forwards.
	// They usually don't need state, but may include state if the
	// there was a new event that references an event that we don't
	// have a copy of.
	KindNew = 2
	// KindBackfill event extend the contiguous graph going backwards.
	// They always have state.
	KindBackfill = 3
)

// DoNotSendToOtherServers tells us not to send the event to other matrix
// servers.
const DoNotSendToOtherServers = ""

// InputRoomEvent is a matrix room event to add to the room server database.
// TODO: Implement UnmarshalJSON/MarshalJSON in a way that does something sensible with the event JSON.
type InputRoomEvent struct {
	// Whether this event is new, backfilled or an outlier.
	// This controls how the event is processed.
	Kind int `json:"kind"`
	// The event JSON for the event to add.
	Event gomatrixserverlib.Event `json:"event"`
	// List of state event IDs that authenticate this event.
	// These are likely derived from the "auth_events" JSON key of the event.
	// But can be different because the "auth_events" key can be incomplete or wrong.
	// For example many matrix events forget to reference the m.room.create event even though it is needed for auth.
	// (since synapse allows this to happen we have to allow it as well.)
	AuthEventIDs []string `json:"auth_event_ids"`
	// Whether the state is supplied as a list of event IDs or whether it
	// should be derived from the state at the previous events.
	HasState bool `json:"has_state"`
	// Optional list of state event IDs forming the state before this event.
	// These state events must have already been persisted.
	// These are only used if HasState is true.
	// The list can be empty, for example when storing the first event in a room.
	StateEventIDs []string `json:"state_event_ids"`
	// The server name to use to push this event to other servers.
	// Or empty if this event shouldn't be pushed to other servers.
	SendAsServer string `json:"send_as_server"`
	// The transaction ID of the send request if sent by a local user and one
	// was specified
	TransactionID *TransactionID `json:"transaction_id"`
}

// TransactionID contains the transaction ID sent by a client when sending an
// event, along with the ID of that device.
type TransactionID struct {
	DeviceID      string `json:"device_id"`
	TransactionID string `json:"id"`
}

// InputInviteEvent is a matrix invite event received over federation without
// the usual context a matrix room event would have. We usually do not have
// access to the events needed to check the event auth rules for the invite.
type InputInviteEvent struct {
	Event gomatrixserverlib.Event `json:"event"`
}

// InputRoomEventsRequest is a request to InputRoomEvents
type InputRoomEventsRequest struct {
	InputRoomEvents   []InputRoomEvent   `json:"input_room_events"`
	InputInviteEvents []InputInviteEvent `json:"input_invite_events"`
}

// InputRoomEventsResponse is a response to InputRoomEvents
type InputRoomEventsResponse struct {
	EventID string `json:"event_id"`
}

// RoomserverInputAPI is used to write events to the room server.
type RoomserverInputAPI interface {
	InputRoomEvents(
		ctx context.Context,
		request *InputRoomEventsRequest,
		response *InputRoomEventsResponse,
	) error
}

// RoomserverInputRoomEventsPath is the HTTP path for the InputRoomEvents API.
const RoomserverInputRoomEventsPath = "/api/roomserver/inputRoomEvents"

// NewRoomserverInputAPIHTTP creates a RoomserverInputAPI implemented by talking to a HTTP POST API.
// If httpClient is nil then it uses the http.DefaultClient
func NewRoomserverInputAPIHTTP(roomserverURL string, httpClient *http.Client) RoomserverInputAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpRoomserverInputAPI{roomserverURL, httpClient}
}

type httpRoomserverInputAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

// InputRoomEvents implements RoomserverInputAPI
func (h *httpRoomserverInputAPI) InputRoomEvents(
	ctx context.Context,
	request *InputRoomEventsRequest,
	response *InputRoomEventsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputRoomEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverInputRoomEventsPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
