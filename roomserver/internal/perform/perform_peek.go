// Copyright 2020-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package perform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	fsAPI "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/internal/input"
	"github.com/element-hq/dendrite/roomserver/storage"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type Peeker struct {
	ServerName spec.ServerName
	Cfg        *config.RoomServer
	FSAPI      fsAPI.RoomserverFederationAPI
	DB         storage.Database

	Inputer *input.Inputer
}

// PerformPeek handles peeking into matrix rooms, including over federation by talking to the federationapi.
func (r *Peeker) PerformPeek(
	ctx context.Context,
	req *api.PerformPeekRequest,
) (roomID string, err error) {
	return r.performPeek(ctx, req)
}

func (r *Peeker) performPeek(
	ctx context.Context,
	req *api.PerformPeekRequest,
) (string, error) {
	// FIXME: there's way too much duplication with performJoin
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return "", api.ErrInvalidID{Err: fmt.Errorf("supplied user ID %q in incorrect format", req.UserID)}
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return "", api.ErrInvalidID{Err: fmt.Errorf("user %q does not belong to this homeserver", req.UserID)}
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "!") {
		return r.performPeekRoomByID(ctx, req)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "#") {
		return r.performPeekRoomByAlias(ctx, req)
	}
	return "", api.ErrInvalidID{Err: fmt.Errorf("room ID or alias %q is invalid", req.RoomIDOrAlias)}
}

func (r *Peeker) performPeekRoomByAlias(
	ctx context.Context,
	req *api.PerformPeekRequest,
) (string, error) {
	// Get the domain part of the room alias.
	_, domain, err := gomatrixserverlib.SplitID('#', req.RoomIDOrAlias)
	if err != nil {
		return "", api.ErrInvalidID{Err: fmt.Errorf("alias %q is not in the correct format", req.RoomIDOrAlias)}
	}
	req.ServerNames = append(req.ServerNames, domain)

	// Check if this alias matches our own server configuration. If it
	// doesn't then we'll need to try a federated peek.
	var roomID string
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		// The alias isn't owned by us, so we will need to try peeking using
		// a remote server.
		dirReq := fsAPI.PerformDirectoryLookupRequest{
			RoomAlias:  req.RoomIDOrAlias, // the room alias to lookup
			ServerName: domain,            // the server to ask
		}
		dirRes := fsAPI.PerformDirectoryLookupResponse{}
		err = r.FSAPI.PerformDirectoryLookup(ctx, &dirReq, &dirRes)
		if err != nil {
			logrus.WithError(err).Errorf("error looking up alias %q", req.RoomIDOrAlias)
			return "", fmt.Errorf("looking up alias %q over federation failed: %w", req.RoomIDOrAlias, err)
		}
		roomID = dirRes.RoomID
		req.ServerNames = append(req.ServerNames, dirRes.ServerNames...)
	} else {
		// Otherwise, look up if we know this room alias locally.
		roomID, err = r.DB.GetRoomIDForAlias(ctx, req.RoomIDOrAlias)
		if err != nil {
			return "", fmt.Errorf("lookup room alias %q failed: %w", req.RoomIDOrAlias, err)
		}
	}

	// If the room ID is empty then we failed to look up the alias.
	if roomID == "" {
		return "", fmt.Errorf("alias %q not found", req.RoomIDOrAlias)
	}

	// If we do, then pluck out the room ID and continue the peek.
	req.RoomIDOrAlias = roomID
	return r.performPeekRoomByID(ctx, req)
}

func (r *Peeker) performPeekRoomByID(
	ctx context.Context,
	req *api.PerformPeekRequest,
) (roomID string, err error) {
	roomID = req.RoomIDOrAlias

	// Get the domain part of the room ID.
	_, domain, err := gomatrixserverlib.SplitID('!', roomID)
	if err != nil {
		return "", api.ErrInvalidID{Err: fmt.Errorf("room ID %q is invalid: %w", roomID, err)}
	}

	// handle federated peeks
	// FIXME: don't create an outbound peek if we already have one going.
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		// If the server name in the room ID isn't ours then it's a
		// possible candidate for finding the room via federation. Add
		// it to the list of servers to try.
		req.ServerNames = append(req.ServerNames, domain)

		// Try peeking by all of the supplied server names.
		fedReq := fsAPI.PerformOutboundPeekRequest{
			RoomID:      req.RoomIDOrAlias, // the room ID to try and peek
			ServerNames: req.ServerNames,   // the servers to try peeking via
		}
		fedRes := fsAPI.PerformOutboundPeekResponse{}
		_ = r.FSAPI.PerformOutboundPeek(ctx, &fedReq, &fedRes)
		if fedRes.LastError != nil {
			return "", fedRes.LastError
		}
	}

	// If this room isn't world_readable, we reject.
	// XXX: would be nicer to call this with NIDs
	// XXX: we should probably factor out history_visibility checks into a common utility method somewhere
	// which handles the default value etc.
	var worldReadable = false
	if ev, _ := r.DB.GetStateEvent(ctx, roomID, "m.room.history_visibility", ""); ev != nil {
		content := map[string]string{}
		if err = json.Unmarshal(ev.Content(), &content); err != nil {
			util.GetLogger(ctx).WithError(err).Error("json.Unmarshal for history visibility failed")
			return "", err
		}
		if visibility, ok := content["history_visibility"]; ok {
			worldReadable = visibility == "world_readable"
		}
	}

	if !worldReadable {
		return "", api.ErrNotAllowed{Err: fmt.Errorf("room is not world-readable")}
	}

	if ev, _ := r.DB.GetStateEvent(ctx, roomID, "m.room.encryption", ""); ev != nil {
		return "", api.ErrNotAllowed{Err: fmt.Errorf("Cannot peek into an encrypted room")}
	}

	// TODO: handle federated peeks

	err = r.Inputer.OutputProducer.ProduceRoomEvents(roomID, []api.OutputEvent{
		{
			Type: api.OutputTypeNewPeek,
			NewPeek: &api.OutputNewPeek{
				RoomID:   roomID,
				UserID:   req.UserID,
				DeviceID: req.DeviceID,
			},
		},
	})
	if err != nil {
		return "", err
	}

	// By this point, if req.RoomIDOrAlias contained an alias, then
	// it will have been overwritten with a room ID by performPeekRoomByAlias.
	// We should now include this in the response so that the CS API can
	// return the right room ID.
	return roomID, nil
}
