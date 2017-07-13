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
	"encoding/json"
	"fmt"
	"net/http"

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

// RoomserverQueryAPI is used to query information from the room server.
type RoomserverQueryAPI interface {
	// Query the latest events and state for a room from the room server.
	QueryLatestEventsAndState(
		request *QueryLatestEventsAndStateRequest,
		response *QueryLatestEventsAndStateResponse,
	) error

	// Query the state after a list of events in a room from the room server.
	QueryStateAfterEvents(
		request *QueryStateAfterEventsRequest,
		response *QueryStateAfterEventsResponse,
	) error

	// Query a list of events by event ID.
	QueryEventsByID(
		request *QueryEventsByIDRequest,
		response *QueryEventsByIDResponse,
	) error
}

// RoomserverQueryLatestEventsAndStatePath is the HTTP path for the QueryLatestEventsAndState API.
const RoomserverQueryLatestEventsAndStatePath = "/api/roomserver/queryLatestEventsAndState"

// RoomserverQueryStateAfterEventsPath is the HTTP path for the QueryStateAfterEvents API.
const RoomserverQueryStateAfterEventsPath = "/api/roomserver/queryStateAfterEvents"

// RoomserverQueryEventsByIDPath is the HTTP path for the QueryEventsByID API.
const RoomserverQueryEventsByIDPath = "/api/roomserver/queryEventsByID"

// NewRoomserverQueryAPIHTTP creates a RoomserverQueryAPI implemented by talking to a HTTP POST API.
// If httpClient is nil then it uses the http.DefaultClient
func NewRoomserverQueryAPIHTTP(roomserverURL string, httpClient *http.Client) RoomserverQueryAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpRoomserverQueryAPI{roomserverURL, *httpClient}
}

type httpRoomserverQueryAPI struct {
	roomserverURL string
	httpClient    http.Client
}

// QueryLatestEventsAndState implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryLatestEventsAndState(
	request *QueryLatestEventsAndStateRequest,
	response *QueryLatestEventsAndStateResponse,
) error {
	apiURL := h.roomserverURL + RoomserverQueryLatestEventsAndStatePath
	return postJSON(h.httpClient, apiURL, request, response)
}

// QueryStateAfterEvents implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryStateAfterEvents(
	request *QueryStateAfterEventsRequest,
	response *QueryStateAfterEventsResponse,
) error {
	apiURL := h.roomserverURL + RoomserverQueryStateAfterEventsPath
	return postJSON(h.httpClient, apiURL, request, response)
}

// QueryEventsByID implements RoomserverQueryAPI
func (h *httpRoomserverQueryAPI) QueryEventsByID(
	request *QueryEventsByIDRequest,
	response *QueryEventsByIDResponse,
) error {
	apiURL := h.roomserverURL + RoomserverQueryEventsByIDPath
	return postJSON(h.httpClient, apiURL, request, response)
}

func postJSON(httpClient http.Client, apiURL string, request, response interface{}) error {
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}
	res, err := httpClient.Post(apiURL, "application/json", bytes.NewReader(jsonBytes))
	if res != nil {
		defer res.Body.Close()
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
