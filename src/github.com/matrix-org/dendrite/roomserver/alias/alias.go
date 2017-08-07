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

package alias

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RoomserverAliasAPIDatabase has the storage APIs needed to implement the alias API.
type RoomserverAliasAPIDatabase interface {
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

// RoomserverAliasAPI is an implementation of api.RoomserverAliasAPI
type RoomserverAliasAPI struct {
	DB       RoomserverAliasAPIDatabase
	Cfg      *config.Dendrite
	InputAPI api.RoomserverInputAPI
	QueryAPI api.RoomserverQueryAPI
}

// SetRoomAlias implements api.RoomserverAliasAPI
func (r *RoomserverAliasAPI) SetRoomAlias(
	request *api.SetRoomAliasRequest,
	response *api.SetRoomAliasResponse,
) error {
	// Check if the alias isn't already referring to a room
	roomID, err := r.DB.GetRoomIDFromAlias(request.Alias)
	if err != nil {
		return err
	}
	if len(roomID) > 0 {
		// If the alias already exists, stop the process
		response.AliasExists = true
		return nil
	}
	response.AliasExists = false

	// Save the new alias
	if err := r.DB.SetRoomAlias(request.Alias, request.RoomID); err != nil {
		return err
	}

	// Send a m.room.aliases event with the updated list of aliases for this room
	if err := r.sendUpdatedAliasesEvent(request.UserID, request.RoomID); err != nil {
		return err
	}

	return nil
}

// GetAliasRoomID implements api.RoomserverAliasAPI
func (r *RoomserverAliasAPI) GetAliasRoomID(
	request *api.GetAliasRoomIDRequest,
	response *api.GetAliasRoomIDResponse,
) error {
	// Lookup the room ID in the database
	roomID, err := r.DB.GetRoomIDFromAlias(request.Alias)
	if err != nil {
		return err
	}

	response.RoomID = roomID
	return nil
}

// RemoveRoomAlias implements api.RoomserverAliasAPI
func (r *RoomserverAliasAPI) RemoveRoomAlias(
	request *api.RemoveRoomAliasRequest,
	response *api.RemoveRoomAliasResponse,
) error {
	// Lookup the room ID in the database
	roomID, err := r.DB.GetRoomIDFromAlias(request.Alias)
	if err != nil {
		return err
	}

	// Remove the dalias from the database
	if err := r.DB.RemoveRoomAlias(request.Alias); err != nil {
		return err
	}

	// Send an updated m.room.aliases event
	if err := r.sendUpdatedAliasesEvent(request.UserID, roomID); err != nil {
		return err
	}

	return nil
}

type roomAliasesContent struct {
	Aliases []string `json:"aliases"`
}

// Build the updated m.room.aliases event to send to the room after addition or
// removal of an alias
func (r *RoomserverAliasAPI) sendUpdatedAliasesEvent(userID string, roomID string) error {
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
	if err = r.QueryAPI.QueryLatestEventsAndState(&req, &res); err != nil {
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

// SetupHTTP adds the RoomserverAliasAPI handlers to the http.ServeMux.
func (r *RoomserverAliasAPI) SetupHTTP(servMux *http.ServeMux) {
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
	servMux.Handle(
		api.RoomserverGetAliasRoomIDPath,
		common.MakeAPI("getAliasRoomID", func(req *http.Request) util.JSONResponse {
			var request api.GetAliasRoomIDRequest
			var response api.GetAliasRoomIDResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetAliasRoomID(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverRemoveRoomAliasPath,
		common.MakeAPI("removeRoomAlias", func(req *http.Request) util.JSONResponse {
			var request api.RemoveRoomAliasRequest
			var response api.RemoveRoomAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.RemoveRoomAlias(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
}
