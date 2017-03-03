package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/matrix-org/gomatrixserverlib"
	"net/http"
)

// StateKeyTuple is a pair of an event type and state_key.
type StateKeyTuple struct {
	EventType     string
	EventStateKey string
}

// QueryLatestEventsAndStateRequest is a request to QueryLatestEventsAndState
type QueryLatestEventsAndStateRequest struct {
	// The roomID to query the latest events for.
	RoomID string
	// The state key tuples to fetch from the room current state.
	// If this list is empty or nil then no events are returned.
	StateToFetch []StateKeyTuple
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

// RoomserverQueryAPI is used to query information from the room server.
type RoomserverQueryAPI interface {
	// Query the latest events and state for a room from the room server.
	QueryLatestEventsAndState(
		request *QueryLatestEventsAndStateRequest,
		response *QueryLatestEventsAndStateResponse,
	) error
}

// RoomserverQueryLatestEventsAndStatePath is the HTTP path for the QueryLatestEventsAndState API.
const RoomserverQueryLatestEventsAndStatePath = "/api/Roomserver/QueryLatestEventsAndState"

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
