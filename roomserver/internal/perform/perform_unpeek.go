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
	"fmt"
	"strings"

	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

type Unpeeker struct {
	ServerName gomatrixserverlib.ServerName
	Cfg        *config.RoomServer
	FSAPI      fsAPI.RoomserverFederationAPI
	DB         storage.Database

	Inputer *input.Inputer
}

// PerformPeek handles peeking into matrix rooms, including over federation by talking to the federationapi.
func (r *Unpeeker) PerformUnpeek(
	ctx context.Context,
	req *api.PerformUnpeekRequest,
	res *api.PerformUnpeekResponse,
) error {
	if err := r.performUnpeek(ctx, req); err != nil {
		perr, ok := err.(*api.PerformError)
		if ok {
			res.Error = perr
		} else {
			res.Error = &api.PerformError{
				Msg: err.Error(),
			}
		}
	}
	return nil
}

func (r *Unpeeker) performUnpeek(
	ctx context.Context,
	req *api.PerformUnpeekRequest,
) error {
	// FIXME: there's way too much duplication with performJoin
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Supplied user ID %q in incorrect format", req.UserID),
		}
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("User %q does not belong to this homeserver", req.UserID),
		}
	}
	if strings.HasPrefix(req.RoomID, "!") {
		return r.performUnpeekRoomByID(ctx, req)
	}
	return &api.PerformError{
		Code: api.PerformErrorBadRequest,
		Msg:  fmt.Sprintf("Room ID %q is invalid", req.RoomID),
	}
}

func (r *Unpeeker) performUnpeekRoomByID(
	_ context.Context,
	req *api.PerformUnpeekRequest,
) (err error) {
	// Get the domain part of the room ID.
	_, _, err = gomatrixserverlib.SplitID('!', req.RoomID)
	if err != nil {
		return &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Room ID %q is invalid: %s", req.RoomID, err),
		}
	}

	// TODO: handle federated peeks

	err = r.Inputer.OutputProducer.ProduceRoomEvents(req.RoomID, []api.OutputEvent{
		{
			Type: api.OutputTypeRetirePeek,
			RetirePeek: &api.OutputRetirePeek{
				RoomID:   req.RoomID,
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
	return nil
}
