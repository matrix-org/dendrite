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
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"

	asAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/util"
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

	return nil
}

// GetRoomIDForAlias implements alias.RoomserverInternalAPI
func (r *RoomserverInternalAPI) GetRoomIDForAlias(
	ctx context.Context,
	request *api.GetRoomIDForAliasRequest,
	response *api.GetRoomIDForAliasResponse,
) error {
	// Look up the room ID in the database
	roomID, err := r.DB.GetRoomIDForAlias(ctx, request.Alias)
	if err == nil && roomID != "" {
		response.RoomID = roomID
		return nil
	}

	// Check appservice on err, but only if the appservice API is
	// wired in and no room ID was found.
	if r.asAPI != nil && request.IncludeAppservices && roomID == "" {
		// No room found locally, try our application services by making a call to
		// the appservice component
		aliasReq := &asAPI.RoomAliasExistsRequest{
			Alias: request.Alias,
		}
		aliasRes := &asAPI.RoomAliasExistsResponse{}
		if err = r.asAPI.RoomAliasExists(ctx, aliasReq, aliasRes); err != nil {
			return err
		}

		if aliasRes.AliasExists {
			roomID, err = r.DB.GetRoomIDForAlias(ctx, request.Alias)
			if err != nil {
				return err
			}
			response.RoomID = roomID
			return nil
		}
	}

	return err
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
	roomID, err := r.DB.GetRoomIDForAlias(ctx, request.Alias)
	if err != nil {
		return fmt.Errorf("r.DB.GetRoomIDForAlias: %w", err)
	}
	response.RoomID = roomID
	if roomID == "" {
		response.Found = false
		response.Removed = false
		return nil
	}

	response.Found = true
	creatorID, err := r.DB.GetCreatorIDForAlias(ctx, request.Alias)
	if err != nil {
		return fmt.Errorf("r.DB.GetCreatorIDForAlias: %w", err)
	}

	if creatorID != request.UserID {
		plEvent, err := r.DB.GetStateEvent(ctx, roomID, gomatrixserverlib.MRoomPowerLevels, "")
		if err != nil {
			return fmt.Errorf("r.DB.GetStateEvent: %w", err)
		}

		pls, err := plEvent.PowerLevels()
		if err != nil {
			return fmt.Errorf("plEvent.PowerLevels: %w", err)
		}

		if pls.UserLevel(request.UserID) < pls.EventLevel(gomatrixserverlib.MRoomCanonicalAlias, true) {
			response.Removed = false
			return nil
		}
	}

	// Remove the alias from the database
	if err := r.DB.RemoveRoomAlias(ctx, request.Alias); err != nil {
		return err
	}

	response.Removed = true

	// If the alias removed is one of the alt_aliases or the canonical one,
	// we need to also remove it from the canonical_alias event
	_ = updateCanonicalAlias(context.TODO(), r, request.UserID, roomID, request.Alias)
	return nil
}

type roomAliasesContent struct {
	Alias   string   `json:"alias,omitempty"`
	Aliases []string `json:"alt_aliases,omitempty"`
}

// Build the updated m.room.aliases event to send to the room after addition or
// removal of an alias
func updateCanonicalAlias(
	ctx context.Context,
	rsAPI *RoomserverInternalAPI,
	userID string,
	roomID string,
	alias string,
) error {
	updated := false
	stateTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCanonicalAlias,
		StateKey:  "",
	}
	stateReq := api.QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{stateTuple},
	}
	stateRes := &api.QueryCurrentStateResponse{}
	err := rsAPI.QueryCurrentState(ctx, &stateReq, stateRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("Query state failed")
		return err
	}

	updatedCanonicalAlias := roomAliasesContent{
		Alias:   "",
		Aliases: []string{""},
	}

	// We try to get the current canonical_alias state, and if found compare its content
	// to the removed alias
	if canonicalAliasEvent, ok := stateRes.StateEvents[stateTuple]; ok {
		canonicalAliasContent := roomAliasesContent{
			Alias:   "",
			Aliases: []string{""},
		}
		// TODO skip malformed event?
		err = json.Unmarshal(canonicalAliasEvent.Content(), &canonicalAliasContent)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("Get canonical_alias event content failed")
			return err
		}
		if alias == canonicalAliasContent.Alias {
			updated = true
		} else {
			updatedCanonicalAlias.Alias = canonicalAliasContent.Alias
		}
		for _, s := range canonicalAliasContent.Aliases {
			if alias == s {
				updated = true
			} else {
				updatedCanonicalAlias.Aliases = append(updatedCanonicalAlias.Aliases, s)
			}
		}
	}

	if !updated {
		return nil
	}

	stateKey := ""

	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     gomatrixserverlib.MRoomCanonicalAlias,
		StateKey: &stateKey,
	}

	rawContent, err := json.Marshal(updatedCanonicalAlias)
	if err != nil {
		return err
	}
	err = builder.SetContent(json.RawMessage(rawContent))
	if err != nil {
		return err
	}
	now := time.Now()
	e, err := eventutil.QueryAndBuildEvent(ctx, &builder, rsAPI.Cfg.Matrix, now, rsAPI, nil)
	if err != nil {
		return err
	}
	err = api.SendEvents(ctx, rsAPI, api.KindNew, []*gomatrixserverlib.HeaderedEvent{e}, rsAPI.Cfg.Matrix.ServerName, nil)
	return err
}
