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

package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomserverInternalAPIDatabase has the storage APIs needed to implement the alias API.
type RoomserverInternalAPIDatabase interface {
	// Save a given room alias with the room ID it refers to.
	// Returns an error if there was a problem talking to the database.
	SetRoomAlias(ctx context.Context, alias string, roomID string, creatorUserID string) error
	// Look up the room ID a given alias refers to.
	// Returns an error if there was a problem talking to the database.
	GetRoomIDForAlias(ctx context.Context, alias string) (string, error)
	// Look up all aliases referring to a given room ID.
	// Returns an error if there was a problem talking to the database.
	GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error)
	// Get the user ID of the creator of an alias.
	// Returns an error if there was a problem talking to the database.
	GetCreatorIDForAlias(ctx context.Context, alias string) (string, error)
	// Remove a given room alias.
	// Returns an error if there was a problem talking to the database.
	RemoveRoomAlias(ctx context.Context, alias string) error
	// Look up the room version for a given room.
	GetRoomVersionForRoom(
		ctx context.Context, roomID string,
	) (gomatrixserverlib.RoomVersion, error)
}

// SetRoomAlias implements alias.RoomserverInternalAPI
func (r *RoomserverInternalAPI) SetRoomAlias(
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
	if err := r.DB.SetRoomAlias(ctx, request.Alias, request.RoomID, request.UserID); err != nil {
		return err
	}

	// Send a m.room.aliases event with the updated list of aliases for this room
	// At this point we've already committed the alias to the database so we
	// shouldn't cancel this request.
	// TODO: Ensure that we send unsent events when if server restarts.
	return r.sendUpdatedAliasesEvent(context.TODO(), request.UserID, request.RoomID)
}

// GetRoomIDForAlias implements alias.RoomserverInternalAPI
func (r *RoomserverInternalAPI) GetRoomIDForAlias(
	ctx context.Context,
	request *api.GetRoomIDForAliasRequest,
	response *api.GetRoomIDForAliasResponse,
) error {
	// Look up the room ID in the database
	roomID, err := r.DB.GetRoomIDForAlias(ctx, request.Alias)
	if err != nil {
		return err
	}

	/*
		TODO: Why is this here? It creates an unnecessary dependency
		from the roomserver to the appservice component, which should be
		altogether optional.

		if roomID == "" {
			// No room found locally, try our application services by making a call to
			// the appservice component
			aliasReq := appserviceAPI.RoomAliasExistsRequest{Alias: request.Alias}
			var aliasResp appserviceAPI.RoomAliasExistsResponse
			if err = r.AppserviceAPI.RoomAliasExists(ctx, &aliasReq, &aliasResp); err != nil {
				return err
			}

			if aliasResp.AliasExists {
				roomID, err = r.DB.GetRoomIDForAlias(ctx, request.Alias)
				if err != nil {
					return err
				}
			}
		}
	*/

	response.RoomID = roomID
	return nil
}

// GetAliasesForRoomID implements alias.RoomserverInternalAPI
func (r *RoomserverInternalAPI) GetAliasesForRoomID(
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

// GetCreatorIDForAlias implements alias.RoomserverInternalAPI
func (r *RoomserverInternalAPI) GetCreatorIDForAlias(
	ctx context.Context,
	request *api.GetCreatorIDForAliasRequest,
	response *api.GetCreatorIDForAliasResponse,
) error {
	// Look up the aliases in the database for the given RoomID
	creatorID, err := r.DB.GetCreatorIDForAlias(ctx, request.Alias)
	if err != nil {
		return err
	}

	response.UserID = creatorID
	return nil
}

// RemoveRoomAlias implements alias.RoomserverInternalAPI
func (r *RoomserverInternalAPI) RemoveRoomAlias(
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
func (r *RoomserverInternalAPI) sendUpdatedAliasesEvent(
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
	if len(eventsNeeded.Tuples()) == 0 {
		return errors.New("expecting state tuples for event builder, got none")
	}
	req := api.QueryLatestEventsAndStateRequest{
		RoomID:       roomID,
		StateToFetch: eventsNeeded.Tuples(),
	}
	var res api.QueryLatestEventsAndStateResponse
	if err = r.QueryLatestEventsAndState(ctx, &req, &res); err != nil {
		return err
	}
	builder.Depth = res.Depth
	builder.PrevEvents = res.LatestEvents

	// Add auth events
	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	for i := range res.StateEvents {
		err = authEvents.AddEvent(&res.StateEvents[i].Event)
		if err != nil {
			return err
		}
	}
	refs, err := eventsNeeded.AuthEventReferences(&authEvents)
	if err != nil {
		return err
	}
	builder.AuthEvents = refs

	roomInfo, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return err
	}
	if roomInfo == nil {
		return fmt.Errorf("room %s does not exist", roomID)
	}

	// Build the event
	now := time.Now()
	event, err := builder.Build(
		now, r.Cfg.Matrix.ServerName, r.Cfg.Matrix.KeyID,
		r.Cfg.Matrix.PrivateKey, roomInfo.RoomVersion,
	)
	if err != nil {
		return err
	}

	// Create the request
	ire := api.InputRoomEvent{
		Kind:         api.KindNew,
		Event:        event.Headered(roomInfo.RoomVersion),
		AuthEventIDs: event.AuthEventIDs(),
		SendAsServer: serverName,
	}
	inputReq := api.InputRoomEventsRequest{
		InputRoomEvents: []api.InputRoomEvent{ire},
	}
	var inputRes api.InputRoomEventsResponse

	// Send the request
	r.InputRoomEvents(ctx, &inputReq, &inputRes)
	return inputRes.Err()
}
