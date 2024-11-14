// Copyright 2020-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package perform

import (
	"context"
	"fmt"
	"strings"

	fsAPI "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/internal/input"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type Unpeeker struct {
	ServerName spec.ServerName
	Cfg        *config.RoomServer
	FSAPI      fsAPI.RoomserverFederationAPI
	Inputer    *input.Inputer
}

// PerformUnpeek handles un-peeking matrix rooms, including over federation by talking to the federationapi.
func (r *Unpeeker) PerformUnpeek(
	ctx context.Context,
	roomID, userID, deviceID string,
) error {
	// FIXME: there's way too much duplication with performJoin
	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return api.ErrInvalidID{Err: fmt.Errorf("supplied user ID %q in incorrect format", userID)}
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return api.ErrInvalidID{Err: fmt.Errorf("user %q does not belong to this homeserver", userID)}
	}
	if strings.HasPrefix(roomID, "!") {
		return r.performUnpeekRoomByID(ctx, roomID, userID, deviceID)
	}
	return api.ErrInvalidID{Err: fmt.Errorf("room ID %q is invalid", roomID)}
}

func (r *Unpeeker) performUnpeekRoomByID(
	_ context.Context,
	roomID, userID, deviceID string,
) (err error) {
	// Get the domain part of the room ID.
	_, _, err = gomatrixserverlib.SplitID('!', roomID)
	if err != nil {
		return api.ErrInvalidID{Err: fmt.Errorf("room ID %q is invalid: %w", roomID, err)}
	}

	// TODO: handle federated peeks
	// By this point, if req.RoomIDOrAlias contained an alias, then
	// it will have been overwritten with a room ID by performPeekRoomByAlias.
	// We should now include this in the response so that the CS API can
	// return the right room ID.
	return r.Inputer.OutputProducer.ProduceRoomEvents(roomID, []api.OutputEvent{
		{
			Type: api.OutputTypeRetirePeek,
			RetirePeek: &api.OutputRetirePeek{
				RoomID:   roomID,
				UserID:   userID,
				DeviceID: deviceID,
			},
		},
	})
}
