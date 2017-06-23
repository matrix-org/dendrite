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

package query

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"net/http"
)

// RoomserverQueryAPIDatabase has the storage APIs needed to implement the query API.
type RoomserverQueryAPIDatabase interface {
	state.RoomStateDatabase
	// Lookup the numeric ID for the room.
	// Returns 0 if the room doesn't exists.
	// Returns an error if there was a problem talking to the database.
	RoomNID(roomID string) (types.RoomNID, error)
	// Lookup event references for the latest events in the room and the current state snapshot.
	// Returns an error if there was a problem talking to the database.
	LatestEventIDs(roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, int64, error)
	// Lookup the numeric IDs for a list of events.
	// Returns an error if there was a problem talking to the database.
	EventNIDs(eventIDs []string) (map[string]types.EventNID, error)
}

// RoomserverQueryAPI is an implementation of RoomserverQueryAPI
type RoomserverQueryAPI struct {
	DB RoomserverQueryAPIDatabase
}

// QueryLatestEventsAndState implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryLatestEventsAndState(
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	response.QueryLatestEventsAndStateRequest = *request
	roomNID, err := r.DB.RoomNID(request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true
	var currentStateSnapshotNID types.StateSnapshotNID
	response.LatestEvents, currentStateSnapshotNID, response.Depth, err = r.DB.LatestEventIDs(roomNID)
	if err != nil {
		return err
	}

	// Lookup the currrent state for the requested tuples.
	stateEntries, err := state.LoadStateAtSnapshotForStringTuples(r.DB, currentStateSnapshotNID, request.StateToFetch)
	if err != nil {
		return err
	}

	stateEvents, err := r.loadStateEvents(stateEntries)
	if err != nil {
		return err
	}

	response.StateEvents = stateEvents
	return nil
}

// QueryStateAfterEvents implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryStateAfterEvents(
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	response.QueryStateAfterEventsRequest = *request
	roomNID, err := r.DB.RoomNID(request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true

	prevStates, err := r.DB.StateAtEventIDs(request.PrevEventIDs)
	if err != nil {
		switch err.(type) {
		case types.MissingEventError:
			return nil
		default:
			return err
		}
	}
	response.PrevEventsExist = true

	// Lookup the currrent state for the requested tuples.
	stateEntries, err := state.LoadStateAfterEventsForStringTuples(r.DB, prevStates, request.StateToFetch)
	if err != nil {
		return err
	}

	stateEvents, err := r.loadStateEvents(stateEntries)
	if err != nil {
		return err
	}

	response.StateEvents = stateEvents
	return nil
}

// QueryEventsByID implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryEventsByID(
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	response.QueryEventsByIDRequest = *request

	eventNIDMap, err := r.DB.EventNIDs(request.EventIDs)
	if err != nil {
		return err
	}

	var eventNIDs []types.EventNID
	for _, nid := range eventNIDMap {
		eventNIDs = append(eventNIDs, nid)
	}

	events, err := r.loadEvents(eventNIDs)
	if err != nil {
		return err
	}

	response.Events = events
	return nil
}

func (r *RoomserverQueryAPI) loadStateEvents(stateEntries []types.StateEntry) ([]gomatrixserverlib.Event, error) {
	eventNIDs := make([]types.EventNID, len(stateEntries))
	for i := range stateEntries {
		eventNIDs[i] = stateEntries[i].EventNID
	}
	return r.loadEvents(eventNIDs)
}

func (r *RoomserverQueryAPI) loadEvents(eventNIDs []types.EventNID) ([]gomatrixserverlib.Event, error) {
	stateEvents, err := r.DB.Events(eventNIDs)
	if err != nil {
		return nil, err
	}

	result := make([]gomatrixserverlib.Event, len(stateEvents))
	for i := range stateEvents {
		result[i] = stateEvents[i].Event
	}
	return result, nil
}

// SetupHTTP adds the RoomserverQueryAPI handlers to the http.ServeMux.
func (r *RoomserverQueryAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.RoomserverQueryLatestEventsAndStatePath,
		common.MakeAPI("query_latest_events_and_state", func(req *http.Request) util.JSONResponse {
			var request api.QueryLatestEventsAndStateRequest
			var response api.QueryLatestEventsAndStateResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryLatestEventsAndState(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryStateAfterEventsPath,
		common.MakeAPI("query_state_after_events", func(req *http.Request) util.JSONResponse {
			var request api.QueryStateAfterEventsRequest
			var response api.QueryStateAfterEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryStateAfterEvents(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryEventsByIDPath,
		common.MakeAPI("query_events_by_id", func(req *http.Request) util.JSONResponse {
			var request api.QueryEventsByIDRequest
			var response api.QueryEventsByIDResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryEventsByID(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
}
