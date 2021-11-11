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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	rsAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type Joiner struct {
	ServerName gomatrixserverlib.ServerName
	Cfg        *config.RoomServer
	FSAPI      fsAPI.FederationSenderInternalAPI
	RSAPI      rsAPI.RoomserverInternalAPI
	DB         storage.Database

	Inputer *input.Inputer
	Queryer *query.Queryer
}

// PerformJoin handles joining matrix rooms, including over federation by talking to the federationsender.
func (r *Joiner) PerformJoin(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
	res *rsAPI.PerformJoinResponse,
) {
	roomID, joinedVia, err := r.performJoin(ctx, req)
	if err != nil {
		sentry.CaptureException(err)
		perr, ok := err.(*rsAPI.PerformError)
		if ok {
			res.Error = perr
		} else {
			res.Error = &rsAPI.PerformError{
				Code: api.PerformErrorNotAllowed, // TODO: fix this when cross-boundary handling is better.
				Msg:  err.Error(),
			}
		}
	}
	res.RoomID = roomID
	res.JoinedVia = joinedVia
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
	if domain != r.Cfg.Matrix.ServerName {
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

// nolint:gocyclo
func (r *Joiner) performJoinRoomByID(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (string, gomatrixserverlib.ServerName, error) {
	// The original client request ?server_name=... may include this HS so filter that out so we
	// don't attempt to make_join with ourselves
	for i := 0; i < len(req.ServerNames); i++ {
		if req.ServerNames[i] == r.Cfg.Matrix.ServerName {
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
		return "", "", fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// It is possible for the request to include some "content" for the
	// event. We'll always overwrite the "membership" key, but the rest,
	// like "display_name" or "avatar_url", will be kept if supplied.
	if req.Content == nil {
		req.Content = map[string]interface{}{}
	}
	req.Content["membership"] = gomatrixserverlib.Join
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
	isInvitePending, inviteSender, _, err := helpers.IsInvitePending(ctx, r.DB, req.RoomIDOrAlias, req.UserID)
	if err == nil && isInvitePending {
		_, inviterDomain, ierr := gomatrixserverlib.SplitID('@', inviteSender)
		if ierr != nil {
			return "", "", fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
		}

		// If we were invited by someone from another server then we can
		// assume they are in the room so we can join via them.
		if inviterDomain != r.Cfg.Matrix.ServerName {
			req.ServerNames = append(req.ServerNames, inviterDomain)
			forceFederatedJoin = true
		}
	}

	// If we should do a forced federated join then do that.
	var joinedVia gomatrixserverlib.ServerName
	if forceFederatedJoin {
		joinedVia, err = r.performFederatedJoinRoomByID(ctx, req)
		return req.RoomIDOrAlias, joinedVia, err
	}

	// Check if the room is a restricted room. If so, update the event
	// builder content. If we can validate that we have a user in one of
	// the restricted rooms then populate 'join_authorised_via_users_server',
	// which will allow the event to pass event auth. If we can't then we
	// leave the event as it is, which will fail auth.
	if restricted, roomIDs, rerr := r.checkIfRestrictedJoin(ctx, req); rerr != nil {
		return "", "", err
	} else if restricted {
		for _, roomID := range roomIDs {
			if err = r.attemptRestrictedJoinUsingRoomID(ctx, req, roomID, &eb); err == nil {
				break
			}
		}
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
			inputReq := rsAPI.InputRoomEventsRequest{
				InputRoomEvents: []rsAPI.InputRoomEvent{
					{
						Kind:         rsAPI.KindNew,
						Event:        event.Headered(buildRes.RoomVersion),
						AuthEventIDs: event.AuthEventIDs(),
						SendAsServer: string(r.Cfg.Matrix.ServerName),
					},
				},
			}
			inputRes := rsAPI.InputRoomEventsResponse{}
			r.Inputer.InputRoomEvents(ctx, &inputReq, &inputRes)
			if err = inputRes.Err(); err != nil {
				return "", "", &rsAPI.PerformError{
					Code: rsAPI.PerformErrorNotAllowed,
					Msg:  fmt.Sprintf("Failed to join the room: %s", err),
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
				return "", "", &rsAPI.PerformError{
					Code: rsAPI.PerformErrorNoRoom,
					Msg:  fmt.Sprintf("Room ID %q does not exist!", req.RoomIDOrAlias),
				}
			}
		}

		// Perform a federated room join.
		joinedVia, err = r.performFederatedJoinRoomByID(ctx, req)
		return req.RoomIDOrAlias, joinedVia, err

	default:
		// Something else went wrong.
		return "", "", &rsAPI.PerformError{
			Code: rsAPI.PerformErrorNotAllowed,
			Msg:  fmt.Sprintf("Failed to join the room: %s", err),
		}
	}

	// By this point, if req.RoomIDOrAlias contained an alias, then
	// it will have been overwritten with a room ID by performJoinRoomByAlias.
	// We should now include this in the response so that the CS API can
	// return the right room ID.
	return req.RoomIDOrAlias, r.Cfg.Matrix.ServerName, nil
}

func (r *Joiner) checkIfRestrictedJoin(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
) (bool, []string, error) {
	// Look up the join rules event for the room, so we can check if it is a
	// restricted room or not.
	joinRuleEvent, err := r.DB.GetStateEvent(ctx, req.RoomIDOrAlias, gomatrixserverlib.MRoomJoinRules, "")
	if err != nil {
		return false, nil, &rsAPI.PerformError{
			Code: rsAPI.PerformErrorNotAllowed,
			Msg:  fmt.Sprintf("Unable to retrieve the join rules: %s", err),
		}
	}

	// First unmarshal the join rule itself. It might seem strange that this is
	// a two-step process, but the Complement tests specifically populate the
	// 'allow' field with a nonsense value that won't unmarshal and therefore
	// trying to unmarshal a gomatrixserverlib.JoinRuleContent fails entirely.
	// We need to get the join rule first to check if the room is restricted
	// though, regardless of what the 'allow' key contains.
	joinRule := struct {
		JoinRule string `json:"join_rule"`
	}{
		JoinRule: gomatrixserverlib.Public,
	}
	if err = json.Unmarshal(joinRuleEvent.Content(), &joinRule); err != nil {
		return false, nil, &rsAPI.PerformError{
			Code: rsAPI.PerformErrorNotAllowed,
			Msg:  fmt.Sprintf("The room join rules are invalid: %s", err),
		}
	}
	if joinRule.JoinRule != gomatrixserverlib.Restricted {
		return false, nil, nil
	}

	// Then try and extract the join rule 'allow' key. It's possible that this
	// step can fail but we need to be OK with that — if we do, we will just
	// treat it as if it is an empty list.
	var joinRuleAllow struct {
		Allow []gomatrixserverlib.JoinRuleContentAllowRule `json:"allow"`
	}
	_ = json.Unmarshal(joinRuleEvent.Content(), &joinRuleAllow)

	// Now create a list of room IDs that we can check in order to validate
	// that the restricted join can be completed.
	roomIDs := make([]string, 0, len(joinRuleAllow.Allow))
	for _, allowed := range joinRuleAllow.Allow {
		if allowed.Type != gomatrixserverlib.MRoomMembership {
			continue
		}
		roomIDs = append(roomIDs, allowed.RoomID)
	}
	return true, roomIDs, nil
}

func (r *Joiner) attemptRestrictedJoinUsingRoomID(
	ctx context.Context,
	req *rsAPI.PerformJoinRequest,
	roomID string,
	eb *gomatrixserverlib.EventBuilder,
) error {
	// Dig out information from the room, including the power levels and
	// our local members joined to that room.
	roomInfo, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return fmt.Errorf("r.DB.RoomInfo: %w", err)
	}
	powerLevelEvent, err := r.DB.GetStateEvent(ctx, roomID, gomatrixserverlib.MRoomPowerLevels, "")
	if err != nil {
		return fmt.Errorf("r.DB.GetStateEvent: %w", err)
	}
	powerLevels, err := powerLevelEvent.PowerLevels()
	if err != nil {
		return fmt.Errorf("powerLevelEvent.PowerLevels: %w", err)
	}
	eventNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, roomInfo.RoomNID, true, true)
	if err != nil {
		return fmt.Errorf("r.DB.GetMembershipEventNIDsForRoom: %w", err)
	}
	events, err := r.DB.Events(ctx, eventNIDs)
	if err != nil {
		return fmt.Errorf("r.DB.Events: %w", err)
	}

	// First of all, look and see if the joining user is joined to the
	// allowed room. If they aren't then there's no point in doing anything
	// else.
	foundInAllowedRoom := false
	for _, event := range events {
		userID := *event.StateKey()
		if userID == req.UserID {
			foundInAllowedRoom = true
			break
		}
	}
	if !foundInAllowedRoom {
		return fmt.Errorf("the user is not joined to this room")
	}

	// Now that we've confirmed that the user is joined to the allowed
	// room, we now need to try and find an authorising user. This needs
	// to be one of our own users with a power level sufficient to issue
	// invites. If we find one then we can place that user ID into the
	// `join_authorised_via_users_server` field.
	for _, event := range events {
		userID := *event.StateKey()
		if userID == req.UserID {
			continue
		}
		_, domain, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil || domain != r.ServerName {
			continue
		}
		if powerLevels.UserLevel(userID) < powerLevels.Invite {
			continue
		}
		if err := eb.SetContent(map[string]string{
			"membership":                       gomatrixserverlib.Join,
			"join_authorised_via_users_server": userID,
		}); err != nil {
			return fmt.Errorf("eb.SetContent: %w", err)
		}
		return nil
	}

	// If we've reached this point then we don't have any of our own
	// users in the room able to issue invites, so we need to give up
	// and hope that we have a suitable user in another room (if any).
	return fmt.Errorf("no suitable power level users found in the room")
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

func buildEvent(
	ctx context.Context, db storage.Database, cfg *config.Global, builder *gomatrixserverlib.EventBuilder,
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
		return nil, nil, fmt.Errorf("QueryLatestEventsAndState: %w", err)
	}

	ev, err := eventutil.BuildEvent(ctx, builder, cfg, time.Now(), &eventsNeeded, &queryRes)
	if err != nil {
		return nil, nil, err
	}
	return ev, &queryRes, nil
}
