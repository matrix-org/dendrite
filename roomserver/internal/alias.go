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
	"database/sql"
	"errors"
	"fmt"
	"time"

	asAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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

// nolint:gocyclo
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
	if roomID == "" {
		response.Found = false
		response.Removed = false
		return nil
	}

	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return err
	}

	sender, err := r.QueryUserIDForSender(ctx, *validRoomID, request.SenderID)
	if err != nil || sender == nil {
		return fmt.Errorf("r.QueryUserIDForSender: %w", err)
	}
	virtualHost := sender.Domain()

	response.Found = true
	creatorID, err := r.DB.GetCreatorIDForAlias(ctx, request.Alias)
	if err != nil {
		return fmt.Errorf("r.DB.GetCreatorIDForAlias: %w", err)
	}

	if spec.SenderID(creatorID) != request.SenderID {
		var plEvent *types.HeaderedEvent
		var pls *gomatrixserverlib.PowerLevelContent

		plEvent, err = r.DB.GetStateEvent(ctx, roomID, spec.MRoomPowerLevels, "")
		if err != nil {
			return fmt.Errorf("r.DB.GetStateEvent: %w", err)
		}

		pls, err = plEvent.PowerLevels()
		if err != nil {
			return fmt.Errorf("plEvent.PowerLevels: %w", err)
		}

		if pls.UserLevel(request.SenderID) < pls.EventLevel(spec.MRoomCanonicalAlias, true) {
			response.Removed = false
			return nil
		}
	}

	ev, err := r.DB.GetStateEvent(ctx, roomID, spec.MRoomCanonicalAlias, "")
	if err != nil && err != sql.ErrNoRows {
		return err
	} else if ev != nil {
		stateAlias := gjson.GetBytes(ev.Content(), "alias").Str
		// the alias to remove is currently set as the canonical alias, remove it
		if stateAlias == request.Alias {
			res, err := sjson.DeleteBytes(ev.Content(), "alias")
			if err != nil {
				return err
			}

			senderID := request.SenderID
			if request.SenderID != ev.SenderID() {
				senderID = ev.SenderID()
			}
			sender, err := r.QueryUserIDForSender(ctx, *validRoomID, senderID)
			if err != nil || sender == nil {
				return err
			}

			senderDomain := sender.Domain()

			identity, err := r.Cfg.Global.SigningIdentityFor(senderDomain)
			if err != nil {
				return err
			}

			proto := &gomatrixserverlib.ProtoEvent{
				SenderID: string(senderID),
				RoomID:   ev.RoomID(),
				Type:     ev.Type(),
				StateKey: ev.StateKey(),
				Content:  res,
			}

			eventsNeeded, err := gomatrixserverlib.StateNeededForProtoEvent(proto)
			if err != nil {
				return fmt.Errorf("gomatrixserverlib.StateNeededForEventBuilder: %w", err)
			}
			if len(eventsNeeded.Tuples()) == 0 {
				return errors.New("expecting state tuples for event builder, got none")
			}

			stateRes := &api.QueryLatestEventsAndStateResponse{}
			if err = helpers.QueryLatestEventsAndState(ctx, r.DB, r, &api.QueryLatestEventsAndStateRequest{RoomID: roomID, StateToFetch: eventsNeeded.Tuples()}, stateRes); err != nil {
				return err
			}

			newEvent, err := eventutil.BuildEvent(ctx, proto, identity, time.Now(), &eventsNeeded, stateRes)
			if err != nil {
				return err
			}

			err = api.SendEvents(ctx, r, api.KindNew, []*types.HeaderedEvent{newEvent}, virtualHost, r.ServerName, r.ServerName, nil, false)
			if err != nil {
				return err
			}
		}
	}

	// Remove the alias from the database
	if err := r.DB.RemoveRoomAlias(ctx, request.Alias); err != nil {
		return err
	}

	response.Removed = true
	return nil
}
