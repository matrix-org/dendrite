// Copyright 2020 New Vector Ltd
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

package perform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type Peeker struct {
	ServerName gomatrixserverlib.ServerName
	Cfg        *config.RoomServer
	FSAPI      fsAPI.RoomserverFederationAPI
	DB         storage.Database

	Inputer *input.Inputer
}

// PerformPeek handles peeking into matrix rooms, including over federation by talking to the federationapi.
func (r *Peeker) PerformPeek(
	ctx context.Context,
	req *api.PerformPeekRequest,
	res *api.PerformPeekResponse,
) error {
	roomID, err := r.performPeek(ctx, req)
	if err != nil {
		perr, ok := err.(*api.PerformError)
		if ok {
			res.Error = perr
		} else {
			res.Error = &api.PerformError{
				Msg: err.Error(),
			}
		}
	}
	res.RoomID = roomID
	return nil
}

func (r *Peeker) performPeek(
	ctx context.Context,
	req *api.PerformPeekRequest,
) (string, error) {
	// FIXME: there's way too much duplication with performJoin
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Supplied user ID %q in incorrect format", req.UserID),
		}
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("User %q does not belong to this homeserver", req.UserID),
		}
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "!") {
		return r.performPeekRoomByID(ctx, req)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "#") {
		return r.performPeekRoomByAlias(ctx, req)
	}
	return "", &api.PerformError{
		Code: api.PerformErrorBadRequest,
		Msg:  fmt.Sprintf("Room ID or alias %q is invalid", req.RoomIDOrAlias),
	}
}

func (r *Peeker) performPeekRoomByAlias(
	ctx context.Context,
	req *api.PerformPeekRequest,
) (string, error) {
	// Get the domain part of the room alias.
	_, domain, err := gomatrixserverlib.SplitID('#', req.RoomIDOrAlias)
	if err != nil {
		return "", fmt.Errorf("alias %q is not in the correct format", req.RoomIDOrAlias)
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
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Room ID %q is invalid: %s", roomID, err),
		}
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
			return "", &api.PerformError{
				Code:       api.PerformErrRemote,
				Msg:        fedRes.LastError.Message,
				RemoteCode: fedRes.LastError.Code,
			}
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
			return
		}
		if visibility, ok := content["history_visibility"]; ok {
			worldReadable = visibility == "world_readable"
		}
	}

	if !worldReadable {
		return "", &api.PerformError{
			Code: api.PerformErrorNotAllowed,
			Msg:  "Room is not world-readable",
		}
	}

	if ev, _ := r.DB.GetStateEvent(ctx, roomID, "m.room.encryption", ""); ev != nil {
		return "", &api.PerformError{
			Code: api.PerformErrorNotAllowed,
			Msg:  "Cannot peek into an encrypted room",
		}
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
		return
	}

	// By this point, if req.RoomIDOrAlias contained an alias, then
	// it will have been overwritten with a room ID by performPeekRoomByAlias.
	// We should now include this in the response so that the CS API can
	// return the right room ID.
	return roomID, nil
}
