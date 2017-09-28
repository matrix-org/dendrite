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

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/opentracing/opentracing-go/ext"

	"github.com/opentracing/opentracing-go"

	"github.com/matrix-org/gomatrixserverlib"
)

// QueryLatestEventsAndStateRequest is a request to QueryLatestEventsAndState
type QueryLatestEventsAndStateRequest struct {
	// The room ID to query the latest events for.
	RoomID string `json:"room_id"`
	// The state key tuples to fetch from the room current state.
	// If this list is empty or nil then no state events are returned.
	StateToFetch []gomatrixserverlib.StateKeyTuple `json:"state_to_fetch"`
}

// QueryLatestEventsAndStateResponse is a response to QueryLatestEventsAndState
// This is used when sending events to set the prev_events, auth_events and depth.
// It is also used to tell whether the event is allowed by the event auth rules.
type QueryLatestEventsAndStateResponse struct {
	// Copy of the request for debugging.
	QueryLatestEventsAndStateRequest
	// Does the room exist?
	// If the room doesn't exist this will be false and LatestEvents will be empty.
	RoomExists bool `json:"room_exists"`
	// The latest events in the room.
	// These are used to set the prev_events when sending an event.
	LatestEvents []gomatrixserverlib.EventReference `json:"latest_events"`
	// The state events requested.
	// This list will be in an arbitrary order.
	// These are used to set the auth_events when sending an event.
	// These are used to check whether the event is allowed.
	StateEvents []gomatrixserverlib.Event `json:"state_events"`
	// The depth of the latest events.
	// This is one greater than the maximum depth of the latest events.
	// This is used to set the depth when sending an event.
	Depth int64 `json:"depth"`
}

// QueryStateAfterEventsRequest is a request to QueryStateAfterEvents
type QueryStateAfterEventsRequest struct {
	// The room ID to query the state in.
	RoomID string `json:"room_id"`
	// The list of previous events to return the events after.
	PrevEventIDs []string `json:"prev_event_ids"`
	// The state key tuples to fetch from the state
	StateToFetch []gomatrixserverlib.StateKeyTuple `json:"state_to_fetch"`
}

// QueryStateAfterEventsResponse is a response to QueryStateAfterEvents
type QueryStateAfterEventsResponse struct {
	// Copy of the request for debugging.
	QueryStateAfterEventsRequest
	// Does the room exist on this roomserver?
	// If the room doesn't exist this will be false and StateEvents will be empty.
	RoomExists bool `json:"room_exists"`
	// Do all the previous events exist on this roomserver?
	// If some of previous events do not exist this will be false and StateEvents will be empty.
	PrevEventsExist bool `json:"prev_events_exist"`
	// The state events requested.
	// This list will be in an arbitrary order.
	StateEvents []gomatrixserverlib.Event `json:"state_events"`
}

// QueryEventsByIDRequest is a request to QueryEventsByID
type QueryEventsByIDRequest struct {
	// The event IDs to look up.
	EventIDs []string `json:"event_ids"`
}

// QueryEventsByIDResponse is a response to QueryEventsByID
type QueryEventsByIDResponse struct {
	// Copy of the request for debugging.
	QueryEventsByIDRequest
	// A list of events with the requested IDs.
	// If the roomserver does not have a copy of a requested event
	// then it will omit that event from the list.
	// If the roomserver thinks it has a copy of the event, but
	// fails to read it from the database then it will fail
	// the entire request.
	// This list will be in an arbitrary order.
	Events []gomatrixserverlib.Event `json:"events"`
}

// QueryMembershipsForRoomRequest is a request to QueryMembershipsForRoom
type QueryMembershipsForRoomRequest struct {
	// If true, only returns the membership events of "join" membership
	JoinedOnly bool `json:"joined_only"`
	// ID of the room to fetch memberships from
	RoomID string `json:"room_id"`
	// ID of the user sending the request
	Sender string `json:"sender"`
}

// QueryMembershipsForRoomResponse is a response to QueryMembershipsForRoom
type QueryMembershipsForRoomResponse struct {
	// The "m.room.member" events (of "join" membership) in the client format
	JoinEvents []gomatrixserverlib.ClientEvent `json:"join_events"`
	// True if the user has been in room before and has either stayed in it or
	// left it.
	HasBeenInRoom bool `json:"has_been_in_room"`
}

// QueryInvitesForUserRequest is a request to QueryInvitesForUser
type QueryInvitesForUserRequest struct {
	// The room ID to look up invites in.
	RoomID string `json:"room_id"`
	// The User ID to look up invites for.
	TargetUserID string `json:"target_user_id"`
}

// QueryInvitesForUserResponse is a response to QueryInvitesForUser
// This is used when accepting an invite or rejecting a invite to tell which
// remote matrix servers to contact.
type QueryInvitesForUserResponse struct {
	// A list of matrix user IDs for each sender of an active invite targeting
	// the requested user ID.
	InviteSenderUserIDs []string `json:"invite_sender_user_ids"`
}

// QueryServerAllowedToSeeEventRequest is a request to QueryServerAllowedToSeeEvent
type QueryServerAllowedToSeeEventRequest struct {
	// The event ID to look up invites in.
	EventID string `json:"event_id"`
	// The server interested in the event
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
}

// QueryServerAllowedToSeeEventResponse is a response to QueryServerAllowedToSeeEvent
type QueryServerAllowedToSeeEventResponse struct {
	// Wether the server in question is allowed to see the event
	AllowedToSeeEvent bool `json:"can_see_event"`
}

// RoomserverQueryAPI is used to query information from the room server.
type RoomserverQueryAPI interface {
	// Query the latest events and state for a room from the room server.
	QueryLatestEventsAndState(
		ctx context.Context,
		request *QueryLatestEventsAndStateRequest,
		response *QueryLatestEventsAndStateResponse,
	) error

	// Query the state after a list of events in a room from the room server.
	QueryStateAfterEvents(
		ctx context.Context,
		request *QueryStateAfterEventsRequest,
		response *QueryStateAfterEventsResponse,
	) error

	// Query a list of events by event ID.
	QueryEventsByID(
		ctx context.Context,
		request *QueryEventsByIDRequest,
		response *QueryEventsByIDResponse,
	) error

	// Query a list of membership events for a room
	QueryMembershipsForRoom(
		ctx context.Context,
		request *QueryMembershipsForRoomRequest,
		response *QueryMembershipsForRoomResponse,
	) error

	// Query a list of invite event senders for a user in a room.
	QueryInvitesForUser(
		ctx context.Context,
		request *QueryInvitesForUserRequest,
		response *QueryInvitesForUserResponse,
	) error

	// Query whether a server is allowed to see an event
	QueryServerAllowedToSeeEvent(
		ctx context.Context,
		request *QueryServerAllowedToSeeEventRequest,
		response *QueryServerAllowedToSeeEventResponse,
	) error
}

// RoomserverQueryLatestEventsAndStatePath is the HTTP path for the QueryLatestEventsAndState API.
const RoomserverQueryLatestEventsAndStatePath = "/api/roomserver/queryLatestEventsAndState"

// RoomserverQueryStateAfterEventsPath is the HTTP path for the QueryStateAfterEvents API.
const RoomserverQueryStateAfterEventsPath = "/api/roomserver/queryStateAfterEvents"

// RoomserverQueryEventsByIDPath is the HTTP path for the QueryEventsByID API.
const RoomserverQueryEventsByIDPath = "/api/roomserver/queryEventsByID"

// RoomserverQueryMembershipsForRoomPath is the HTTP path for the QueryMembershipsForRoom API
const RoomserverQueryMembershipsForRoomPath = "/api/roomserver/queryMembershipsForRoom"

// RoomserverQueryInvitesForUserPath is the HTTP path for the QueryInvitesForUser API
const RoomserverQueryInvitesForUserPath = "/api/roomserver/queryInvitesForUser"

// RoomserverQueryServerAllowedToSeeEventPath is the HTTP path for the QueryServerAllowedToSeeEvent API
const RoomserverQueryServerAllowedToSeeEventPath = "/api/roomserver/queryServerAllowedToSeeEvent"

// NewRoomserverQueryAPIHTTP creates a RoomserverQueryAPI implemented by talking to a HTTP POST API.
// If httpClient is nil then it uses the http.DefaultClient
func NewRoomserverQueryAPIHTTP(roomserverURL string, httpClient *http.Client) RoomserverQueryAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpRoomserverQueryAPI{roomserverURL, httpClient}
}

type httpRoomserverQueryAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

// QueryLatestEventsAndState implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryLatestEventsAndState(
	ctx context.Context,
	request *QueryLatestEventsAndStateRequest,
	response *QueryLatestEventsAndStateResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryLatestEventsAndState")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryLatestEventsAndStatePath
	return postJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryStateAfterEvents implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *QueryStateAfterEventsRequest,
	response *QueryStateAfterEventsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryStateAfterEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryStateAfterEventsPath
	return postJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryEventsByID implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryEventsByID(
	ctx context.Context,
	request *QueryEventsByIDRequest,
	response *QueryEventsByIDResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryEventsByID")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryEventsByIDPath
	return postJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMembershipsForRoom implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryMembershipsForRoom(
	ctx context.Context,
	request *QueryMembershipsForRoomRequest,
	response *QueryMembershipsForRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMembershipsForRoom")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryMembershipsForRoomPath
	return postJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryInvitesForUser implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryInvitesForUser(
	ctx context.Context,
	request *QueryInvitesForUserRequest,
	response *QueryInvitesForUserResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryInvitesForUser")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryInvitesForUserPath
	return postJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryServerAllowedToSeeEvent implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryServerAllowedToSeeEvent(
	ctx context.Context,
	request *QueryServerAllowedToSeeEventRequest,
	response *QueryServerAllowedToSeeEventResponse,
) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryServerAllowedToSeeEvent")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryServerAllowedToSeeEventPath
	return postJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func postJSON(
	ctx context.Context, span opentracing.Span, httpClient *http.Client,
	apiURL string, request, response interface{},
) error {
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}

	// Mark the span as being an RPC client.
	ext.SpanKindRPCClient.Set(span)
	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	tracer := opentracing.GlobalTracer()

	if err = tracer.Inject(span.Context(), opentracing.HTTPHeaders, carrier); err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := httpClient.Do(req.WithContext(ctx))
	if res != nil {
		defer (func() { err = res.Body.Close() })()
	}
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		var errorBody struct {
			Message string `json:"message"`
		}
		if err = json.NewDecoder(res.Body).Decode(&errorBody); err != nil {
			return err
		}
		return fmt.Errorf("api: %d: %s", res.StatusCode, errorBody.Message)
	}
	return json.NewDecoder(res.Body).Decode(response)
}
