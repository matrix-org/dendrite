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

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	rsAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
)

type Joiner struct {
	Cfg   *config.RoomServer
	FSAPI fsAPI.RoomserverFederationAPI
	RSAPI rsAPI.RoomserverInternalAPI
	DB    storage.Database

	Inputer *input.Inputer
	Queryer *query.Queryer
}

// PerformJoin handles joining matrix rooms, including over federation by talking to the federationapi.
func (r *Joiner) PerformJoin(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
	res *rsAPI.PerformJoinResponse,
) error {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"room_id": req.RoomIDOrAlias,
		"user_id": req.UserID,
		"servers": req.ServerNames,
	})
	logger.Info("User requested to room join")
	roomID, joinedVia, err := r.performJoin(context.Background(), req)
	if err != nil {
		logger.WithError(err).Error("Failed to join room")
		sentry.CaptureException(err)
		perr, ok := err.(*rsAPI.PerformError)
		if ok {
			res.Error = perr
		} else {
			res.Error = &rsAPI.PerformError{
				Msg: err.Error(),
			}
		}
		return nil
	}
	logger.Info("User joined room successfully")
	res.RoomID = roomID
	res.JoinedVia = joinedVia
	return nil
}

func (r *Joiner) performJoin(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (string, gomatrixserverlib.ServerName, error) {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return "", "", &rsAPI.PerformError{
			Code: rsAPI.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Supplied user ID %q in incorrect format", req.UserID),
		}
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return "", "", &rsAPI.PerformError{
			Code: rsAPI.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("User %q does not belong to this homeserver", req.UserID),
		}
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "!") {
		return r.performJoinRoomByID(ctx, req)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "#") {
		return r.performJoinRoomByAlias(ctx, req)
	}
	return "", "", &rsAPI.PerformError{
		Code: rsAPI.PerformErrorBadRequest,
		Msg:  fmt.Sprintf("Room ID or alias %q is invalid", req.RoomIDOrAlias),
	}
}

func (r *Joiner) performJoinRoomByAlias(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (string, gomatrixserverlib.ServerName, error) {
	// Get the domain part of the room alias.
	_, domain, err := gomatrixserverlib.SplitID('#', req.RoomIDOrAlias)
	if err != nil {
		return "", "", fmt.Errorf("alias %q is not in the correct format", req.RoomIDOrAlias)
	}
	req.ServerNames = append(req.ServerNames, domain)

	// Check if this alias matches our own server configuration. If it
	// doesn't then we'll need to try a federated join.
	var roomID string
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
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
			return "", "", fmt.Errorf("looking up alias %q over federation failed: %w", req.RoomIDOrAlias, err)
		}
		roomID = dirRes.RoomID
		req.ServerNames = append(req.ServerNames, dirRes.ServerNames...)
	} else {
		var getRoomReq = rsAPI.GetRoomIDForAliasRequest{
			Alias:              req.RoomIDOrAlias,
			IncludeAppservices: true,
		}
		var getRoomRes = rsAPI.GetRoomIDForAliasResponse{}
		// Otherwise, look up if we know this room alias locally.
		err = r.RSAPI.GetRoomIDForAlias(ctx, &getRoomReq, &getRoomRes)
		if err != nil {
			return "", "", fmt.Errorf("lookup room alias %q failed: %w", req.RoomIDOrAlias, err)
		}
		roomID = getRoomRes.RoomID
	}

	// If the room ID is empty then we failed to look up the alias.
	if roomID == "" {
		return "", "", fmt.Errorf("alias %q not found", req.RoomIDOrAlias)
	}

	// If we do, then pluck out the room ID and continue the join.
	req.RoomIDOrAlias = roomID
	return r.performJoinRoomByID(ctx, req)
}

// TODO: Break this function up a bit
// nolint:gocyclo
func (r *Joiner) performJoinRoomByID(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (string, gomatrixserverlib.ServerName, error) {
	// The original client request ?server_name=... may include this HS so filter that out so we
	// don't attempt to make_join with ourselves
	for i := 0; i < len(req.ServerNames); i++ {
		if r.Cfg.Matrix.IsLocalServerName(req.ServerNames[i]) {
			// delete this entry
			req.ServerNames = append(req.ServerNames[:i], req.ServerNames[i+1:]...)
			i--
		}
	}

	// Get the domain part of the room ID.
	_, domain, err := gomatrixserverlib.SplitID('!', req.RoomIDOrAlias)
	if err != nil {
		return "", "", &rsAPI.PerformError{
			Code: rsAPI.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Room ID %q is invalid: %s", req.RoomIDOrAlias, err),
		}
	}

	// If the server name in the room ID isn't ours then it's a
	// possible candidate for finding the room via federation. Add
	// it to the list of servers to try.
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		req.ServerNames = append(req.ServerNames, domain)
	}

	// Prepare the template for the join event.
	userID := req.UserID
	_, userDomain, err := r.Cfg.Matrix.SplitLocalID('@', userID)
	if err != nil {
		return "", "", &rsAPI.PerformError{
			Code: rsAPI.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("User ID %q is invalid: %s", userID, err),
		}
	}
	eb := gomatrixserverlib.EventBuilder{
		Type:     gomatrixserverlib.MRoomMember,
		Sender:   userID,
		StateKey: &userID,
		RoomID:   req.RoomIDOrAlias,
		Redacts:  "",
	}
	if err = eb.SetUnsigned(struct{}{}); err != nil {
		return "", "", fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// It is possible for the request to include some "content" for the
	// event. We'll always overwrite the "membership" key, but the rest,
	// like "display_name" or "avatar_url", will be kept if supplied.
	if req.Content == nil {
		req.Content = map[string]interface{}{}
	}
	req.Content["membership"] = gomatrixserverlib.Join
	if authorisedVia, aerr := r.populateAuthorisedViaUserForRestrictedJoin(ctx, req); aerr != nil {
		return "", "", aerr
	} else if authorisedVia != "" {
		req.Content["join_authorised_via_users_server"] = authorisedVia
	}
	if err = eb.SetContent(req.Content); err != nil {
		return "", "", fmt.Errorf("eb.SetContent: %w", err)
	}

	// Force a federated join if we aren't in the room and we've been
	// given some server names to try joining by.
	inRoomReq := &rsAPI.QueryServerJoinedToRoomRequest{
		RoomID: req.RoomIDOrAlias,
	}
	inRoomRes := &rsAPI.QueryServerJoinedToRoomResponse{}
	if err = r.Queryer.QueryServerJoinedToRoom(ctx, inRoomReq, inRoomRes); err != nil {
		return "", "", fmt.Errorf("r.Queryer.QueryServerJoinedToRoom: %w", err)
	}
	serverInRoom := inRoomRes.IsInRoom
	forceFederatedJoin := len(req.ServerNames) > 0 && !serverInRoom

	// Force a federated join if we're dealing with a pending invite
	// and we aren't in the room.
	isInvitePending, inviteSender, _, inviteEvent, err := helpers.IsInvitePending(ctx, r.DB, req.RoomIDOrAlias, req.UserID)
	if err == nil && !serverInRoom && isInvitePending {
		_, inviterDomain, ierr := gomatrixserverlib.SplitID('@', inviteSender)
		if ierr != nil {
			return "", "", fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
		}

		// If we were invited by someone from another server then we can
		// assume they are in the room so we can join via them.
		if !r.Cfg.Matrix.IsLocalServerName(inviterDomain) {
			req.ServerNames = append(req.ServerNames, inviterDomain)
			forceFederatedJoin = true
			memberEvent := gjson.Parse(string(inviteEvent.JSON()))
			// only set unsigned if we've got a content.membership, which we _should_
			if memberEvent.Get("content.membership").Exists() {
				req.Unsigned = map[string]interface{}{
					"prev_sender": memberEvent.Get("sender").Str,
					"prev_content": map[string]interface{}{
						"is_direct":  memberEvent.Get("content.is_direct").Bool(),
						"membership": memberEvent.Get("content.membership").Str,
					},
				}
			}
		}
	}

	// If we should do a forced federated join then do that.
	var joinedVia gomatrixserverlib.ServerName
	if forceFederatedJoin {
		joinedVia, err = r.performFederatedJoinRoomByID(ctx, req)
		return req.RoomIDOrAlias, joinedVia, err
	}

	// Try to construct an actual join event from the template.
	// If this succeeds then it is a sign that the room already exists
	// locally on the homeserver.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.
	event, buildRes, err := buildEvent(ctx, r.DB, r.Cfg.Matrix, userDomain, &eb)

	switch err {
	case nil:
		// The room join is local. Send the new join event into the
		// roomserver. First of all check that the user isn't already
		// a member of the room. This is best-effort (as in we won't
		// fail if we can't find the existing membership) because there
		// is really no harm in just sending another membership event.
		membershipReq := &api.QueryMembershipForUserRequest{
			RoomID: req.RoomIDOrAlias,
			UserID: userID,
		}
		membershipRes := &api.QueryMembershipForUserResponse{}
		_ = r.Queryer.QueryMembershipForUser(ctx, membershipReq, membershipRes)

		// If we haven't already joined the room then send an event
		// into the room changing our membership status.
		if !membershipRes.RoomExists || !membershipRes.IsInRoom {
			inputReq := rsAPI.InputRoomEventsRequest{
				InputRoomEvents: []rsAPI.InputRoomEvent{
					{
						Kind:         rsAPI.KindNew,
						Event:        event.Headered(buildRes.RoomVersion),
						SendAsServer: string(userDomain),
					},
				},
			}
			inputRes := rsAPI.InputRoomEventsResponse{}
			if err = r.Inputer.InputRoomEvents(ctx, &inputReq, &inputRes); err != nil {
				return "", "", &rsAPI.PerformError{
					Code: rsAPI.PerformErrorNoOperation,
					Msg:  fmt.Sprintf("InputRoomEvents failed: %s", err),
				}
			}
			if err = inputRes.Err(); err != nil {
				return "", "", &rsAPI.PerformError{
					Code: rsAPI.PerformErrorNotAllowed,
					Msg:  fmt.Sprintf("InputRoomEvents auth failed: %s", err),
				}
			}
		}

	case eventutil.ErrRoomNoExists:
		// The room doesn't exist locally. If the room ID looks like it should
		// be ours then this probably means that we've nuked our database at
		// some point.
		if r.Cfg.Matrix.IsLocalServerName(domain) {
			// If there are no more server names to try then give up here.
			// Otherwise we'll try a federated join as normal, since it's quite
			// possible that the room still exists on other servers.
			if len(req.ServerNames) == 0 {
				return "", "", &rsAPI.PerformError{
					Code: rsAPI.PerformErrorNoRoom,
					Msg:  fmt.Sprintf("room ID %q does not exist", req.RoomIDOrAlias),
				}
			}
		}

		// Perform a federated room join.
		joinedVia, err = r.performFederatedJoinRoomByID(ctx, req)
		return req.RoomIDOrAlias, joinedVia, err

	default:
		// Something else went wrong.
		return "", "", fmt.Errorf("error joining local room: %q", err)
	}

	// By this point, if req.RoomIDOrAlias contained an alias, then
	// it will have been overwritten with a room ID by performJoinRoomByAlias.
	// We should now include this in the response so that the CS API can
	// return the right room ID.
	return req.RoomIDOrAlias, userDomain, nil
}

func (r *Joiner) performFederatedJoinRoomByID(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (gomatrixserverlib.ServerName, error) {
	// Try joining by all of the supplied server names.
	fedReq := fsAPI.PerformJoinRequest{
		RoomID:      req.RoomIDOrAlias, // the room ID to try and join
		UserID:      req.UserID,        // the user ID joining the room
		ServerNames: req.ServerNames,   // the server to try joining with
		Content:     req.Content,       // the membership event content
		Unsigned:    req.Unsigned,      // the unsigned event content, if any
	}
	fedRes := fsAPI.PerformJoinResponse{}
	r.FSAPI.PerformJoin(ctx, &fedReq, &fedRes)
	if fedRes.LastError != nil {
		return "", &rsAPI.PerformError{
			Code:       rsAPI.PerformErrRemote,
			Msg:        fedRes.LastError.Message,
			RemoteCode: fedRes.LastError.Code,
		}
	}
	return fedRes.JoinedVia, nil
}

func (r *Joiner) populateAuthorisedViaUserForRestrictedJoin(
	ctx context.Context,
	joinReq *rsAPI.PerformJoinRequest,
) (string, error) {
	req := &api.QueryRestrictedJoinAllowedRequest{
		UserID: joinReq.UserID,
		RoomID: joinReq.RoomIDOrAlias,
	}
	res := &api.QueryRestrictedJoinAllowedResponse{}
	if err := r.Queryer.QueryRestrictedJoinAllowed(ctx, req, res); err != nil {
		return "", fmt.Errorf("r.Queryer.QueryRestrictedJoinAllowed: %w", err)
	}
	if !res.Restricted {
		return "", nil
	}
	if !res.Resident {
		return "", nil
	}
	if !res.Allowed {
		return "", &rsAPI.PerformError{
			Code: rsAPI.PerformErrorNotAllowed,
			Msg:  fmt.Sprintf("The join to room %s was not allowed.", joinReq.RoomIDOrAlias),
		}
	}
	return res.AuthorisedVia, nil
}

func buildEvent(
	ctx context.Context, db storage.Database, cfg *config.Global,
	senderDomain gomatrixserverlib.ServerName,
	builder *gomatrixserverlib.EventBuilder,
) (*gomatrixserverlib.HeaderedEvent, *rsAPI.QueryLatestEventsAndStateResponse, error) {
	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return nil, nil, fmt.Errorf("gomatrixserverlib.StateNeededForEventBuilder: %w", err)
	}

	if len(eventsNeeded.Tuples()) == 0 {
		return nil, nil, errors.New("expecting state tuples for event builder, got none")
	}

	var queryRes rsAPI.QueryLatestEventsAndStateResponse
	err = helpers.QueryLatestEventsAndState(ctx, db, &rsAPI.QueryLatestEventsAndStateRequest{
		RoomID:       builder.RoomID,
		StateToFetch: eventsNeeded.Tuples(),
	}, &queryRes)
	if err != nil {
		switch err.(type) {
		case types.MissingStateError:
			// We know something about the room but the state seems to be
			// insufficient to actually build a new event, so in effect we
			// had might as well treat the room as if it doesn't exist.
			return nil, nil, eventutil.ErrRoomNoExists
		default:
			return nil, nil, fmt.Errorf("QueryLatestEventsAndState: %w", err)
		}
	}

	identity, err := cfg.SigningIdentityFor(senderDomain)
	if err != nil {
		return nil, nil, err
	}

	ev, err := eventutil.BuildEvent(ctx, builder, cfg, identity, time.Now(), &eventsNeeded, &queryRes)
	if err != nil {
		return nil, nil, err
	}
	return ev, &queryRes, nil
}
