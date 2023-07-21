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
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
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
) (roomID string, joinedVia spec.ServerName, err error) {
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"room_id": req.RoomIDOrAlias,
		"user_id": req.UserID,
		"servers": req.ServerNames,
	})
	logger.Info("User requested to room join")
	roomID, joinedVia, err = r.performJoin(context.Background(), req)
	if err != nil {
		logger.WithError(err).Error("Failed to join room")
		sentry.CaptureException(err)
		return "", "", err
	}
	logger.Info("User joined room successfully")

	return roomID, joinedVia, nil
}

func (r *Joiner) performJoin(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (string, spec.ServerName, error) {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return "", "", rsAPI.ErrInvalidID{Err: fmt.Errorf("supplied user ID %q in incorrect format", req.UserID)}
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return "", "", rsAPI.ErrInvalidID{Err: fmt.Errorf("user %q does not belong to this homeserver", req.UserID)}
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "!") {
		return r.performJoinRoomByID(ctx, req)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "#") {
		return r.performJoinRoomByAlias(ctx, req)
	}
	return "", "", rsAPI.ErrInvalidID{Err: fmt.Errorf("room ID or alias %q is invalid", req.RoomIDOrAlias)}
}

func (r *Joiner) performJoinRoomByAlias(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (string, spec.ServerName, error) {
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

// TODO: Break this function up a bit & move to GMSL
// nolint:gocyclo
func (r *Joiner) performJoinRoomByID(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (string, spec.ServerName, error) {
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
	roomID, err := spec.NewRoomID(req.RoomIDOrAlias)
	if err != nil {
		return "", "", rsAPI.ErrInvalidID{Err: fmt.Errorf("room ID %q is invalid: %w", req.RoomIDOrAlias, err)}
	}

	// If the server name in the room ID isn't ours then it's a
	// possible candidate for finding the room via federation. Add
	// it to the list of servers to try.
	if !r.Cfg.Matrix.IsLocalServerName(roomID.Domain()) {
		req.ServerNames = append(req.ServerNames, roomID.Domain())
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

	userID, err := spec.NewUserID(req.UserID, true)
	if err != nil {
		return "", "", rsAPI.ErrInvalidID{Err: fmt.Errorf("user ID %q is invalid: %w", req.UserID, err)}
	}

	// Look up the room NID for the supplied room ID.
	var senderID spec.SenderID
	checkInvitePending := false
	info, err := r.DB.RoomInfo(ctx, req.RoomIDOrAlias)
	if err == nil && info != nil {
		switch info.RoomVersion {
		case gomatrixserverlib.RoomVersionPseudoIDs:
			senderID, err = r.Queryer.QuerySenderIDForUser(ctx, *roomID, *userID)
			if err == nil {
				checkInvitePending = true
			}
			if senderID == "" {
				// create user room key if needed
				key, keyErr := r.RSAPI.GetOrCreateUserRoomPrivateKey(ctx, *userID, *roomID)
				if keyErr != nil {
					util.GetLogger(ctx).WithError(keyErr).Error("GetOrCreateUserRoomPrivateKey failed")
					return "", "", fmt.Errorf("GetOrCreateUserRoomPrivateKey failed: %w", keyErr)
				}
				senderID = spec.SenderIDFromPseudoIDKey(key)
			}
		default:
			checkInvitePending = true
			senderID = spec.SenderID(userID.String())
		}
	}

	userDomain := userID.Domain()

	// Force a federated join if we're dealing with a pending invite
	// and we aren't in the room.
	if checkInvitePending {
		isInvitePending, inviteSender, _, inviteEvent, inviteErr := helpers.IsInvitePending(ctx, r.DB, req.RoomIDOrAlias, senderID)
		if inviteErr == nil && !serverInRoom && isInvitePending {
			inviter, queryErr := r.RSAPI.QueryUserIDForSender(ctx, *roomID, inviteSender)
			if queryErr != nil {
				return "", "", fmt.Errorf("r.RSAPI.QueryUserIDForSender: %w", queryErr)
			}

			// If we were invited by someone from another server then we can
			// assume they are in the room so we can join via them.
			if inviter != nil && !r.Cfg.Matrix.IsLocalServerName(inviter.Domain()) {
				req.ServerNames = append(req.ServerNames, inviter.Domain())
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
	}

	// If a guest is trying to join a room, check that the room has a m.room.guest_access event
	if req.IsGuest {
		var guestAccessEvent *types.HeaderedEvent
		guestAccess := "forbidden"
		guestAccessEvent, err = r.DB.GetStateEvent(ctx, req.RoomIDOrAlias, spec.MRoomGuestAccess, "")
		if (err != nil && !errors.Is(err, sql.ErrNoRows)) || guestAccessEvent == nil {
			logrus.WithError(err).Warn("unable to get m.room.guest_access event, defaulting to 'forbidden'")
		}
		if guestAccessEvent != nil {
			guestAccess = gjson.GetBytes(guestAccessEvent.Content(), "guest_access").String()
		}

		// Servers MUST only allow guest users to join rooms if the m.room.guest_access state event
		// is present on the room and has the guest_access value can_join.
		if guestAccess != "can_join" {
			return "", "", rsAPI.ErrNotAllowed{Err: fmt.Errorf("guest access is forbidden")}
		}
	}

	// If we should do a forced federated join then do that.
	var joinedVia spec.ServerName
	if forceFederatedJoin {
		joinedVia, err = r.performFederatedJoinRoomByID(ctx, req)
		return req.RoomIDOrAlias, joinedVia, err
	}

	// Try to construct an actual join event from the template.
	// If this succeeds then it is a sign that the room already exists
	// locally on the homeserver.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.

	var buildRes rsAPI.QueryLatestEventsAndStateResponse
	identity := r.Cfg.Matrix.SigningIdentity

	// at this point we know we have an existing room
	if inRoomRes.RoomVersion == gomatrixserverlib.RoomVersionPseudoIDs {
		var pseudoIDKey ed25519.PrivateKey
		pseudoIDKey, err = r.RSAPI.GetOrCreateUserRoomPrivateKey(ctx, *userID, *roomID)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("GetOrCreateUserRoomPrivateKey failed")
			return "", "", err
		}

		mapping := &gomatrixserverlib.MXIDMapping{
			UserRoomKey: spec.SenderIDFromPseudoIDKey(pseudoIDKey),
			UserID:      userID.String(),
		}

		// Sign the mapping with the server identity
		if err = mapping.Sign(identity.ServerName, identity.KeyID, identity.PrivateKey); err != nil {
			return "", "", err
		}
		req.Content["mxid_mapping"] = mapping

		// sign the event with the pseudo ID key
		identity = fclient.SigningIdentity{
			ServerName: spec.ServerName(spec.SenderIDFromPseudoIDKey(pseudoIDKey)),
			KeyID:      "ed25519:1",
			PrivateKey: pseudoIDKey,
		}
	}

	senderIDString := string(senderID)

	// Prepare the template for the join event.
	proto := gomatrixserverlib.ProtoEvent{
		Type:     spec.MRoomMember,
		SenderID: senderIDString,
		StateKey: &senderIDString,
		RoomID:   req.RoomIDOrAlias,
		Redacts:  "",
	}
	if err = proto.SetUnsigned(struct{}{}); err != nil {
		return "", "", fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// It is possible for the request to include some "content" for the
	// event. We'll always overwrite the "membership" key, but the rest,
	// like "display_name" or "avatar_url", will be kept if supplied.
	if req.Content == nil {
		req.Content = map[string]interface{}{}
	}
	req.Content["membership"] = spec.Join
	if authorisedVia, aerr := r.populateAuthorisedViaUserForRestrictedJoin(ctx, req, senderID); aerr != nil {
		return "", "", aerr
	} else if authorisedVia != "" {
		req.Content["join_authorised_via_users_server"] = authorisedVia
	}
	if err = proto.SetContent(req.Content); err != nil {
		return "", "", fmt.Errorf("eb.SetContent: %w", err)
	}
	event, err := eventutil.QueryAndBuildEvent(ctx, &proto, &identity, time.Now(), r.RSAPI, &buildRes)

	switch err.(type) {
	case nil:
		// The room join is local. Send the new join event into the
		// roomserver. First of all check that the user isn't already
		// a member of the room. This is best-effort (as in we won't
		// fail if we can't find the existing membership) because there
		// is really no harm in just sending another membership event.
		membershipRes := &api.QueryMembershipForUserResponse{}
		_ = r.Queryer.QueryMembershipForSenderID(ctx, *roomID, senderID, membershipRes)

		// If we haven't already joined the room then send an event
		// into the room changing our membership status.
		if !membershipRes.RoomExists || !membershipRes.IsInRoom {
			inputReq := rsAPI.InputRoomEventsRequest{
				InputRoomEvents: []rsAPI.InputRoomEvent{
					{
						Kind:         rsAPI.KindNew,
						Event:        event,
						SendAsServer: string(userDomain),
					},
				},
			}
			inputRes := rsAPI.InputRoomEventsResponse{}
			r.Inputer.InputRoomEvents(ctx, &inputReq, &inputRes)
			if err = inputRes.Err(); err != nil {
				return "", "", rsAPI.ErrNotAllowed{Err: err}
			}
		}

	case eventutil.ErrRoomNoExists:
		// The room doesn't exist locally. If the room ID looks like it should
		// be ours then this probably means that we've nuked our database at
		// some point.
		if r.Cfg.Matrix.IsLocalServerName(roomID.Domain()) {
			// If there are no more server names to try then give up here.
			// Otherwise we'll try a federated join as normal, since it's quite
			// possible that the room still exists on other servers.
			if len(req.ServerNames) == 0 {
				return "", "", eventutil.ErrRoomNoExists{}
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
) (spec.ServerName, error) {
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
		return "", fedRes.LastError
	}
	return fedRes.JoinedVia, nil
}

func (r *Joiner) populateAuthorisedViaUserForRestrictedJoin(
	ctx context.Context,
	joinReq *rsAPI.PerformJoinRequest,
	senderID spec.SenderID,
) (string, error) {
	roomID, err := spec.NewRoomID(joinReq.RoomIDOrAlias)
	if err != nil {
		return "", err
	}

	return r.Queryer.QueryRestrictedJoinAllowed(ctx, *roomID, senderID)
}
