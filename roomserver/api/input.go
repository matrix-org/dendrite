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
	"errors"
	"net/http"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
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
	Event gomatrixserverlib.HeaderedEvent `json:"event"`
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
// event, along with the ID of the client session.
type TransactionID struct {
	SessionID     int64  `json:"session_id"`
	TransactionID string `json:"id"`
}

// InputInviteEvent is a matrix invite event received over federation without
// the usual context a matrix room event would have. We usually do not have
// access to the events needed to check the event auth rules for the invite.
type InputInviteEvent struct {
	RoomVersion     gomatrixserverlib.RoomVersion             `json:"room_version"`
	Event           gomatrixserverlib.HeaderedEvent           `json:"event"`
	InviteRoomState []gomatrixserverlib.InviteV2StrippedState `json:"invite_room_state"`
	SendAsServer    string                                    `json:"send_as_server"`
	TransactionID   *TransactionID                            `json:"transaction_id"`
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
	// needed to avoid chicken and egg scenario when setting up the
	// interdependencies between the roomserver and the FS input API
	SetFederationSenderInputAPI(fsInputAPI fsAPI.FederationSenderInputAPI)
	InputRoomEvents(
		ctx context.Context,
		request *InputRoomEventsRequest,
		response *InputRoomEventsResponse,
	) error
}

// RoomserverInputRoomEventsPath is the HTTP path for the InputRoomEvents API.
const RoomserverInputRoomEventsPath = "/api/roomserver/inputRoomEvents"

// NewRoomserverInputAPIHTTP creates a RoomserverInputAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewRoomserverInputAPIHTTP(roomserverURL string, httpClient *http.Client) (RoomserverInputAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewRoomserverInputAPIHTTP: httpClient is <nil>")
	}
	return &httpRoomserverInputAPI{roomserverURL, httpClient, nil}, nil
}

type httpRoomserverInputAPI struct {
	roomserverURL string
	httpClient    *http.Client
	// The federation sender API allows us to send federation
	// requests from the new perform input requests, still TODO.
	fsInputAPI fsAPI.FederationSenderInputAPI
}

// SetFederationSenderInputAPI passes in a federation sender input API reference
// so that we can avoid the chicken-and-egg problem of both the roomserver input API
// and the federation sender input API being interdependent.
func (h *httpRoomserverInputAPI) SetFederationSenderInputAPI(fsInputAPI fsAPI.FederationSenderInputAPI) {
	h.fsInputAPI = fsInputAPI
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
