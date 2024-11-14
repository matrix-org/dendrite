// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	asAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/internal/helpers"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// SetRoomAlias implements alias.RoomserverInternalAPI
func (r *RoomserverInternalAPI) SetRoomAlias(
	ctx context.Context,
	senderID spec.SenderID,
	roomID spec.RoomID,
	alias string,
) (aliasAlreadyUsed bool, err error) {
	// Check if the alias isn't already referring to a room
	existingRoomID, err := r.DB.GetRoomIDForAlias(ctx, alias)
	if err != nil {
		return false, err
	}

	if len(existingRoomID) > 0 {
		// If the alias already exists, stop the process
		return true, nil
	}

	// Save the new alias
	if err := r.DB.SetRoomAlias(ctx, alias, roomID.String(), string(senderID)); err != nil {
		return false, err
	}

	return false, nil
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
// nolint: gocyclo
func (r *RoomserverInternalAPI) RemoveRoomAlias(ctx context.Context, senderID spec.SenderID, alias string) (aliasFound bool, aliasRemoved bool, err error) {
	roomID, err := r.DB.GetRoomIDForAlias(ctx, alias)
	if err != nil {
		return false, false, fmt.Errorf("r.DB.GetRoomIDForAlias: %w", err)
	}
	if roomID == "" {
		return false, false, nil
	}

	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return true, false, err
	}

	sender, err := r.QueryUserIDForSender(ctx, *validRoomID, senderID)
	if err != nil || sender == nil {
		return true, false, fmt.Errorf("r.QueryUserIDForSender: %w", err)
	}
	virtualHost := sender.Domain()

	creatorID, err := r.DB.GetCreatorIDForAlias(ctx, alias)
	if err != nil {
		return true, false, fmt.Errorf("r.DB.GetCreatorIDForAlias: %w", err)
	}

	if spec.SenderID(creatorID) != senderID {
		var plEvent *types.HeaderedEvent
		var pls *gomatrixserverlib.PowerLevelContent

		plEvent, err = r.DB.GetStateEvent(ctx, roomID, spec.MRoomPowerLevels, "")
		if err != nil {
			return true, false, fmt.Errorf("r.DB.GetStateEvent: %w", err)
		}

		pls, err = plEvent.PowerLevels()
		if err != nil {
			return true, false, fmt.Errorf("plEvent.PowerLevels: %w", err)
		}

		if pls.UserLevel(senderID) < pls.EventLevel(spec.MRoomCanonicalAlias, true) {
			return true, false, nil
		}
	}

	ev, err := r.DB.GetStateEvent(ctx, roomID, spec.MRoomCanonicalAlias, "")
	if err != nil && err != sql.ErrNoRows {
		return true, false, err
	} else if ev != nil {
		stateAlias := gjson.GetBytes(ev.Content(), "alias").Str
		// the alias to remove is currently set as the canonical alias, remove it
		if stateAlias == alias {
			res, err := sjson.DeleteBytes(ev.Content(), "alias")
			if err != nil {
				return true, false, err
			}

			canonicalSenderID := ev.SenderID()
			canonicalSender, err := r.QueryUserIDForSender(ctx, *validRoomID, canonicalSenderID)
			if err != nil || canonicalSender == nil {
				return true, false, err
			}

			validRoomID, err := spec.NewRoomID(roomID)
			if err != nil {
				return true, false, err
			}
			identity, err := r.SigningIdentityFor(ctx, *validRoomID, *canonicalSender)
			if err != nil {
				return true, false, err
			}

			proto := &gomatrixserverlib.ProtoEvent{
				SenderID: string(canonicalSenderID),
				RoomID:   ev.RoomID().String(),
				Type:     ev.Type(),
				StateKey: ev.StateKey(),
				Content:  res,
			}

			eventsNeeded, err := gomatrixserverlib.StateNeededForProtoEvent(proto)
			if err != nil {
				return true, false, fmt.Errorf("gomatrixserverlib.StateNeededForEventBuilder: %w", err)
			}
			if len(eventsNeeded.Tuples()) == 0 {
				return true, false, errors.New("expecting state tuples for event builder, got none")
			}

			stateRes := &api.QueryLatestEventsAndStateResponse{}
			if err = helpers.QueryLatestEventsAndState(ctx, r.DB, r, &api.QueryLatestEventsAndStateRequest{RoomID: roomID, StateToFetch: eventsNeeded.Tuples()}, stateRes); err != nil {
				return true, false, err
			}

			newEvent, err := eventutil.BuildEvent(ctx, proto, &identity, time.Now(), &eventsNeeded, stateRes)
			if err != nil {
				return true, false, err
			}

			err = api.SendEvents(ctx, r, api.KindNew, []*types.HeaderedEvent{newEvent}, virtualHost, r.ServerName, r.ServerName, nil, false)
			if err != nil {
				return true, false, err
			}
		}
	}

	// Remove the alias from the database
	if err := r.DB.RemoveRoomAlias(ctx, alias); err != nil {
		return true, false, err
	}

	return true, true, nil
}
