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
	"context"
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
	SetRoomAlias(ctx context.Context, alias string, roomID string) error
	// Look up the room ID a given alias refers to.
	// Returns an error if there was a problem talking to the database.
	GetRoomIDForAlias(ctx context.Context, alias string) (string, error)
	// Look up all aliases referring to a given room ID.
	// Returns an error if there was a problem talking to the database.
	GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error)
	// Remove a given room alias.
	// Returns an error if there was a problem talking to the database.
	RemoveRoomAlias(ctx context.Context, alias string) error
}

// RoomserverAliasAPI is an implementation of alias.RoomserverAliasAPI
type RoomserverAliasAPI struct {
	DB       RoomserverAliasAPIDatabase
	Cfg      *config.Dendrite
	InputAPI api.RoomserverInputAPI
	QueryAPI api.RoomserverQueryAPI
}

// SetRoomAlias implements alias.RoomserverAliasAPI
func (r *RoomserverAliasAPI) SetRoomAlias(
	ctx context.Context,
	request *api.SetRoomAliasRequest,
	response *api.SetRoomAliasResponse,
) error {
	// Check if the alias isn't already referring to a room
	roomID, err := r.DB.GetRoomIDForAlias(ctx, request.Alias)
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
	if err := r.DB.SetRoomAlias(ctx, request.Alias, request.RoomID); err != nil {
		return err
	}

	// Send a m.room.aliases event with the updated list of aliases for this room
	// At this point we've already committed the alias to the database so we
	// shouldn't cancel this request.
	// TODO: Ensure that we send unsent events when if server restarts.
	return r.sendUpdatedAliasesEvent(context.TODO(), request.UserID, request.RoomID)
}

// GetRoomIDForAlias implements alias.RoomserverAliasAPI
func (r *RoomserverAliasAPI) GetRoomIDForAlias(
	ctx context.Context,
	request *api.GetRoomIDForAliasRequest,
	response *api.GetRoomIDForAliasResponse,
) error {
	// Look up the room ID in the database
	roomID, err := r.DB.GetRoomIDForAlias(ctx, request.Alias)
	if err != nil {
		return err
	}

	response.RoomID = roomID
	return nil
}

// GetAliasesForRoomID implements alias.RoomserverAliasAPI
func (r *RoomserverAliasAPI) GetAliasesForRoomID(
	ctx context.Context,
	request *api.GetAliasesForRoomIDRequest,
	response *api.GetAliasesForRoomIDResponse,
) error {
	// Look up the aliases in the database for the given RoomID
	aliases, err := r.DB.GetAliasesForRoomID(ctx, request.RoomID)
	if err != nil {
		return err
	}

	response.Aliases = aliases
	return nil
}

// RemoveRoomAlias implements alias.RoomserverAliasAPI
func (r *RoomserverAliasAPI) RemoveRoomAlias(
	ctx context.Context,
	request *api.RemoveRoomAliasRequest,
	response *api.RemoveRoomAliasResponse,
) error {
	// Look up the room ID in the database
	roomID, err := r.DB.GetRoomIDForAlias(ctx, request.Alias)
	if err != nil {
		return err
	}

	// Remove the dalias from the database
	if err := r.DB.RemoveRoomAlias(ctx, request.Alias); err != nil {
		return err
	}

	// Send an updated m.room.aliases event
	// At this point we've already committed the alias to the database so we
	// shouldn't cancel this request.
	// TODO: Ensure that we send unsent events when if server restarts.
	return r.sendUpdatedAliasesEvent(context.TODO(), request.UserID, roomID)
}

type roomAliasesContent struct {
	Aliases []string `json:"aliases"`
}

// Build the updated m.room.aliases event to send to the room after addition or
// removal of an alias
func (r *RoomserverAliasAPI) sendUpdatedAliasesEvent(
	ctx context.Context, userID string, roomID string,
) error {
	serverName := string(r.Cfg.Matrix.ServerName)

	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     "m.room.aliases",
		StateKey: &serverName,
	}

	// Retrieve the updated list of aliases, marhal it and set it as the
	// event's content
	aliases, err := r.DB.GetAliasesForRoomID(ctx, roomID)
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
	if err = r.QueryAPI.QueryLatestEventsAndState(ctx, &req, &res); err != nil {
		return err
	}
	builder.Depth = res.Depth
	builder.PrevEvents = res.LatestEvents

	// Add auth events
	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	for i := range res.StateEvents {
		err = authEvents.AddEvent(&res.StateEvents[i])
		if err != nil {
			return err
		}
	}
	refs, err := eventsNeeded.AuthEventReferences(&authEvents)
	if err != nil {
		return err
	}
	builder.AuthEvents = refs

	// Build the event
	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), r.Cfg.Matrix.ServerName)
	now := time.Now()
	event, err := builder.Build(
		eventID, now, r.Cfg.Matrix.ServerName, r.Cfg.Matrix.KeyID, r.Cfg.Matrix.PrivateKey,
	)
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
	return r.InputAPI.InputRoomEvents(ctx, &inputReq, &inputRes)
}

// SetupHTTP adds the RoomserverAliasAPI handlers to the http.ServeMux.
func (r *RoomserverAliasAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.RoomserverSetRoomAliasPath,
		common.MakeInternalAPI("setRoomAlias", func(req *http.Request) util.JSONResponse {
			var request api.SetRoomAliasRequest
			var response api.SetRoomAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.SetRoomAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverGetRoomIDForAliasPath,
		common.MakeInternalAPI("GetRoomIDForAlias", func(req *http.Request) util.JSONResponse {
			var request api.GetRoomIDForAliasRequest
			var response api.GetRoomIDForAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetRoomIDForAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverRemoveRoomAliasPath,
		common.MakeInternalAPI("removeRoomAlias", func(req *http.Request) util.JSONResponse {
			var request api.RemoveRoomAliasRequest
			var response api.RemoveRoomAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.RemoveRoomAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
