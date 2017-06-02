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
	"github.com/matrix-org/gomatrixserverlib"
	"net/http"
)

// QueryLatestEventsAndStateRequest is a request to QueryLatestEventsAndState
type QueryLatestEventsAndStateRequest struct {
	// The room ID to query the latest events for.
	RoomID string
	// The state key tuples to fetch from the room current state.
	// If this list is empty or nil then no state events are returned.
	StateToFetch []gomatrixserverlib.StateKeyTuple
}

// QueryLatestEventsAndStateResponse is a response to QueryLatestEventsAndState
type QueryLatestEventsAndStateResponse struct {
	// Copy of the request for debugging.
	QueryLatestEventsAndStateRequest
	// Does the room exist?
	// If the room doesn't exist this will be false and LatestEvents will be empty.
	RoomExists bool
	// The latest events in the room.
	LatestEvents []gomatrixserverlib.EventReference
	// The state events requested.
	StateEvents []gomatrixserverlib.Event
}

// QueryStateAfterEventsRequest is a request to QueryStateAfterEvents
type QueryStateAfterEventsRequest struct {
	// The room ID to query the state in.
	RoomID string
	// The list of previous events to return the events after.
	PrevEventIDs []string
	// The state key tuples to fetch from the state
	StateToFetch []gomatrixserverlib.StateKeyTuple
}

// QueryStateAfterEventsResponse is a response to QueryStateAfterEvents
type QueryStateAfterEventsResponse struct {
	// Copy of the request for debugging.
	QueryStateAfterEventsRequest
	// Does the room exist on this roomserver?
	// If the room doesn't exist this will be false and StateEvents will be empty.
	RoomExists bool
	// Do all the previous events exist on this roomserver?
	// If some of previous events do not exist this will be false and StateEvents will be empty.
	PrevEventsExist bool
	// The state events requested.
	StateEvents []gomatrixserverlib.Event
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
}

// RoomserverQueryLatestEventsAndStatePath is the HTTP path for the QueryLatestEventsAndState API.
const RoomserverQueryLatestEventsAndStatePath = "/api/roomserver/QueryLatestEventsAndState"

// RoomserverQueryStateAfterEventsPath is the HTTP path for the QueryStateAfterEvents API.
const RoomserverQueryStateAfterEventsPath = "/api/roomserver/QueryStateAfterEvents"

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
