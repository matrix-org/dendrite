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
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/input"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RoomserverQueryAPIDatabase has the storage APIs needed to implement the query API.
type RoomserverQueryAPIDatabase interface {
	state.RoomStateDatabase
	// Lookup the numeric ID for the room.
	// Returns 0 if the room doesn't exists.
	// Returns an error if there was a problem talking to the database.
	RoomNID(roomID string) (types.RoomNID, error)
	// Lookup event references for the latest events in the room and the current state snapshot.
	// Returns the latest events, the current state and the maximum depth of the latest events plus 1.
	// Returns an error if there was a problem talking to the database.
	LatestEventIDs(roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, int64, error)
	// Lookup the numeric IDs for a list of events.
	// Returns an error if there was a problem talking to the database.
	EventNIDs(eventIDs []string) (map[string]types.EventNID, error)
	// Save a given room alias with the room ID it refers to.
	// Returns an error if there was a problem talking to the database.
	SetRoomAlias(alias string, roomID string) error
	// Lookup the room ID a given alias refers to.
	// Returns an error if there was a problem talking to the database.
	GetRoomIDFromAlias(alias string) (string, error)
	// Lookup all aliases referring to a given room ID.
	// Returns an error if there was a problem talking to the database.
	GetAliasesFromRoomID(roomID string) ([]string, error)
	// Remove a given room alias.
	// Returns an error if there was a problem talking to the database.
	RemoveRoomAlias(alias string) error
}

// RoomserverQueryAPI is an implementation of api.RoomserverQueryAPI
type RoomserverQueryAPI struct {
	DB       RoomserverQueryAPIDatabase
	Cfg      *config.Dendrite
	InputAPI input.RoomserverInputAPI
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

// SetRoomAlias implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) SetRoomAlias(
	request *api.SetRoomAliasRequest,
	response *api.SetRoomAliasResponse,
) error {
	// Save the new alias
	// TODO: Check if alias already exists
	if err := r.DB.SetRoomAlias(request.Alias, request.RoomID); err != nil {
		return err
	}

	// Send a m.room.aliases event with the updated list of aliases for this room
	if err := r.sendUpdatedAliasesEvent(request.UserID, request.RoomID); err != nil {
		return err
	}

	// We don't need to return anything in the response, so we don't edit it
	return nil
}

type roomAliasesContent struct {
	Aliases []string `json:"aliases"`
}

// Build the updated m.room.aliases event to send to the room after addition or
// removal of an alias
func (r *RoomserverQueryAPI) sendUpdatedAliasesEvent(userID string, roomID string) error {
	serverName := string(r.Cfg.Matrix.ServerName)

	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     "m.room.aliases",
		StateKey: &serverName,
	}

	// Retrieve the updated list of aliases, marhal it and set it as the
	// event's content
	aliases, err := r.DB.GetAliasesFromRoomID(roomID)
	if err != nil {
		return err
	}
	content := roomAliasesContent{Aliases: aliases}
	rawContent, err := json.Marshal(content)
	if err != nil {
		return err
	}
	err = builder.SetContent(json.RawMessage(rawContent))
	if err != nil {
		return err
	}

	// Get needed state events and depth
	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(&builder)
	if err != nil {
		return err
	}
	req := api.QueryLatestEventsAndStateRequest{
		RoomID:       roomID,
		StateToFetch: eventsNeeded.Tuples(),
	}
	var res api.QueryLatestEventsAndStateResponse
	if err = r.QueryLatestEventsAndState(&req, &res); err != nil {
		return err
	}
	builder.Depth = res.Depth
	builder.PrevEvents = res.LatestEvents

	// Add auth events
	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	for i := range res.StateEvents {
		authEvents.AddEvent(&res.StateEvents[i])
	}
	refs, err := eventsNeeded.AuthEventReferences(&authEvents)
	if err != nil {
		return err
	}
	builder.AuthEvents = refs

	// Build the event
	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), r.Cfg.Matrix.ServerName)
	now := time.Now()
	event, err := builder.Build(eventID, now, r.Cfg.Matrix.ServerName, r.Cfg.Matrix.KeyID, r.Cfg.Matrix.PrivateKey)
	if err != nil {
		return err
	}

	// Create the request
	ire := api.InputRoomEvent{
		Kind:         api.KindNew,
		Event:        event,
		AuthEventIDs: event.AuthEventIDs(),
		SendAsServer: serverName,
	}
	inputReq := api.InputRoomEventsRequest{
		InputRoomEvents: []api.InputRoomEvent{ire},
	}
	var inputRes api.InputRoomEventsResponse

	// Send the request
	if err := r.InputAPI.InputRoomEvents(&inputReq, &inputRes); err != nil {
		return err
	}

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
		common.MakeAPI("queryLatestEventsAndState", func(req *http.Request) util.JSONResponse {
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
		common.MakeAPI("queryStateAfterEvents", func(req *http.Request) util.JSONResponse {
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
		common.MakeAPI("queryEventsByID", func(req *http.Request) util.JSONResponse {
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
	servMux.Handle(
		api.RoomserverSetRoomAliasPath,
		common.MakeAPI("setRoomAlias", func(req *http.Request) util.JSONResponse {
			var request api.SetRoomAliasRequest
			var response api.SetRoomAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.SetRoomAlias(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
}
