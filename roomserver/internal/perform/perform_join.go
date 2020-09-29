// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"errors"
	"fmt"
	"strings"
	"time"

	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type Joiner struct {
	ServerName gomatrixserverlib.ServerName
	Cfg        *config.RoomServer
	FSAPI      fsAPI.FederationSenderInternalAPI
	DB         storage.Database

	Inputer *input.Inputer
}

// PerformJoin handles joining matrix rooms, including over federation by talking to the federationsender.
func (r *Joiner) PerformJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) {
	roomID, err := r.performJoin(ctx, req)
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
}

func (r *Joiner) performJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
) (string, error) {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Supplied user ID %q in incorrect format", req.UserID),
		}
	}
	if domain != r.Cfg.Matrix.ServerName {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("User %q does not belong to this homeserver", req.UserID),
		}
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "!") {
		return r.performJoinRoomByID(ctx, req)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "#") {
		return r.performJoinRoomByAlias(ctx, req)
	}
	return "", &api.PerformError{
		Code: api.PerformErrorBadRequest,
		Msg:  fmt.Sprintf("Room ID or alias %q is invalid", req.RoomIDOrAlias),
	}
}

func (r *Joiner) performJoinRoomByAlias(
	ctx context.Context,
	req *api.PerformJoinRequest,
) (string, error) {
	// Get the domain part of the room alias.
	_, domain, err := gomatrixserverlib.SplitID('#', req.RoomIDOrAlias)
	if err != nil {
		return "", fmt.Errorf("Alias %q is not in the correct format", req.RoomIDOrAlias)
	}
	req.ServerNames = append(req.ServerNames, domain)

	// Check if this alias matches our own server configuration. If it
	// doesn't then we'll need to try a federated join.
	var roomID string
	if domain != r.Cfg.Matrix.ServerName {
		// The alias isn't owned by us, so we will need to try joining using
		// a remote server.
		dirReq := fsAPI.PerformDirectoryLookupRequest{
			RoomAlias:  req.RoomIDOrAlias, // the room alias to lookup
			ServerName: domain,            // the server to ask
		}
		dirRes := fsAPI.PerformDirectoryLookupResponse{}
		err = r.FSAPI.PerformDirectoryLookup(ctx, &dirReq, &dirRes)
		if err != nil {
			logrus.WithError(err).Errorf("error looking up alias %q", req.RoomIDOrAlias)
			return "", fmt.Errorf("Looking up alias %q over federation failed: %w", req.RoomIDOrAlias, err)
		}
		roomID = dirRes.RoomID
		req.ServerNames = append(req.ServerNames, dirRes.ServerNames...)
	} else {
		// Otherwise, look up if we know this room alias locally.
		roomID, err = r.DB.GetRoomIDForAlias(ctx, req.RoomIDOrAlias)
		if err != nil {
			return "", fmt.Errorf("Lookup room alias %q failed: %w", req.RoomIDOrAlias, err)
		}
	}

	// If the room ID is empty then we failed to look up the alias.
	if roomID == "" {
		return "", fmt.Errorf("Alias %q not found", req.RoomIDOrAlias)
	}

	// If we do, then pluck out the room ID and continue the join.
	req.RoomIDOrAlias = roomID
	return r.performJoinRoomByID(ctx, req)
}

// TODO: Break this function up a bit
// nolint:gocyclo
func (r *Joiner) performJoinRoomByID(
	ctx context.Context,
	req *api.PerformJoinRequest,
) (string, error) {
	// Get the domain part of the room ID.
	_, domain, err := gomatrixserverlib.SplitID('!', req.RoomIDOrAlias)
	if err != nil {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Room ID %q is invalid: %s", req.RoomIDOrAlias, err),
		}
	}

	// If the server name in the room ID isn't ours then it's a
	// possible candidate for finding the room via federation. Add
	// it to the list of servers to try.
	if domain != r.Cfg.Matrix.ServerName {
		req.ServerNames = append(req.ServerNames, domain)
	}

	// Prepare the template for the join event.
	userID := req.UserID
	eb := gomatrixserverlib.EventBuilder{
		Type:     gomatrixserverlib.MRoomMember,
		Sender:   userID,
		StateKey: &userID,
		RoomID:   req.RoomIDOrAlias,
		Redacts:  "",
	}
	if err = eb.SetUnsigned(struct{}{}); err != nil {
		return "", fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// It is possible for the request to include some "content" for the
	// event. We'll always overwrite the "membership" key, but the rest,
	// like "display_name" or "avatar_url", will be kept if supplied.
	if req.Content == nil {
		req.Content = map[string]interface{}{}
	}
	req.Content["membership"] = gomatrixserverlib.Join
	if err = eb.SetContent(req.Content); err != nil {
		return "", fmt.Errorf("eb.SetContent: %w", err)
	}

	// Force a federated join if we aren't in the room and we've been
	// given some server names to try joining by.
	serverInRoom, _ := helpers.IsServerCurrentlyInRoom(ctx, r.DB, r.ServerName, req.RoomIDOrAlias)
	forceFederatedJoin := len(req.ServerNames) > 0 && !serverInRoom

	// Force a federated join if we're dealing with a pending invite
	// and we aren't in the room.
	isInvitePending, inviteSender, _, err := helpers.IsInvitePending(ctx, r.DB, req.RoomIDOrAlias, req.UserID)
	if err == nil && isInvitePending {
		_, inviterDomain, ierr := gomatrixserverlib.SplitID('@', inviteSender)
		if ierr != nil {
			return "", fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
		}

		// If we were invited by someone from another server then we can
		// assume they are in the room so we can join via them.
		if inviterDomain != r.Cfg.Matrix.ServerName {
			req.ServerNames = append(req.ServerNames, inviterDomain)
			forceFederatedJoin = true
		}
	}

	// If we should do a forced federated join then do that.
	if forceFederatedJoin {
		return req.RoomIDOrAlias, r.performFederatedJoinRoomByID(ctx, req)
	}

	// Try to construct an actual join event from the template.
	// If this succeeds then it is a sign that the room already exists
	// locally on the homeserver.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.
	event, buildRes, err := buildEvent(ctx, r.DB, r.Cfg.Matrix, &eb)

	switch err {
	case nil:
		// The room join is local. Send the new join event into the
		// roomserver. First of all check that the user isn't already
		// a member of the room.
		alreadyJoined := false
		for _, se := range buildRes.StateEvents {
			if !se.StateKeyEquals(userID) {
				continue
			}
			if membership, merr := se.Membership(); merr == nil {
				alreadyJoined = (membership == gomatrixserverlib.Join)
				break
			}
		}

		// If we haven't already joined the room then send an event
		// into the room changing our membership status.
		if !alreadyJoined {
			inputReq := api.InputRoomEventsRequest{
				InputRoomEvents: []api.InputRoomEvent{
					{
						Kind:         api.KindNew,
						Event:        event.Headered(buildRes.RoomVersion),
						AuthEventIDs: event.AuthEventIDs(),
						SendAsServer: string(r.Cfg.Matrix.ServerName),
					},
				},
			}
			inputRes := api.InputRoomEventsResponse{}
			r.Inputer.InputRoomEvents(ctx, &inputReq, &inputRes)
			if err = inputRes.Err(); err != nil {
				return "", &api.PerformError{
					Code: api.PerformErrorNotAllowed,
					Msg:  fmt.Sprintf("InputRoomEvents auth failed: %s", err),
				}
			}
		}

	case eventutil.ErrRoomNoExists:
		// The room doesn't exist locally. If the room ID looks like it should
		// be ours then this probably means that we've nuked our database at
		// some point.
		if domain == r.Cfg.Matrix.ServerName {
			// If there are no more server names to try then give up here.
			// Otherwise we'll try a federated join as normal, since it's quite
			// possible that the room still exists on other servers.
			if len(req.ServerNames) == 0 {
				return "", &api.PerformError{
					Code: api.PerformErrorNoRoom,
					Msg:  fmt.Sprintf("Room ID %q does not exist", req.RoomIDOrAlias),
				}
			}
		}

		// Perform a federated room join.
		return req.RoomIDOrAlias, r.performFederatedJoinRoomByID(ctx, req)

	default:
		// Something else went wrong.
		return "", fmt.Errorf("Error joining local room: %q", err)
	}

	// By this point, if req.RoomIDOrAlias contained an alias, then
	// it will have been overwritten with a room ID by performJoinRoomByAlias.
	// We should now include this in the response so that the CS API can
	// return the right room ID.
	return req.RoomIDOrAlias, nil
}

func (r *Joiner) performFederatedJoinRoomByID(
	ctx context.Context,
	req *api.PerformJoinRequest,
) error {
	// Try joining by all of the supplied server names.
	fedReq := fsAPI.PerformJoinRequest{
		RoomID:      req.RoomIDOrAlias, // the room ID to try and join
		UserID:      req.UserID,        // the user ID joining the room
		ServerNames: req.ServerNames,   // the server to try joining with
		Content:     req.Content,       // the membership event content
	}
	fedRes := fsAPI.PerformJoinResponse{}
	r.FSAPI.PerformJoin(ctx, &fedReq, &fedRes)
	if fedRes.LastError != nil {
		return &api.PerformError{
			Code:       api.PerformErrRemote,
			Msg:        fedRes.LastError.Message,
			RemoteCode: fedRes.LastError.Code,
		}
	}
	return nil
}

func buildEvent(
	ctx context.Context, db storage.Database, cfg *config.Global, builder *gomatrixserverlib.EventBuilder,
) (*gomatrixserverlib.HeaderedEvent, *api.QueryLatestEventsAndStateResponse, error) {
	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return nil, nil, fmt.Errorf("gomatrixserverlib.StateNeededForEventBuilder: %w", err)
	}

	if len(eventsNeeded.Tuples()) == 0 {
		return nil, nil, errors.New("expecting state tuples for event builder, got none")
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	err = helpers.QueryLatestEventsAndState(ctx, db, &api.QueryLatestEventsAndStateRequest{
		RoomID:       builder.RoomID,
		StateToFetch: eventsNeeded.Tuples(),
	}, &queryRes)
	if err != nil {
		return nil, nil, fmt.Errorf("QueryLatestEventsAndState: %w", err)
	}

	ev, err := eventutil.BuildEvent(ctx, builder, cfg, time.Now(), &eventsNeeded, &queryRes)
	if err != nil {
		return nil, nil, err
	}
	return ev, &queryRes, nil
}
