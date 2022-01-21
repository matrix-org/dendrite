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

package routing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	"github.com/sirupsen/logrus"
)

type invite struct {
	MXID   string                                         `json:"mxid"`
	RoomID string                                         `json:"room_id"`
	Sender string                                         `json:"sender"`
	Token  string                                         `json:"token"`
	Signed gomatrixserverlib.MemberThirdPartyInviteSigned `json:"signed"`
}

type invites struct {
	Medium  string   `json:"medium"`
	Address string   `json:"address"`
	MXID    string   `json:"mxid"`
	Invites []invite `json:"invites"`
}

var (
	errNotLocalUser = errors.New("the user is not from this server")
	errNotInRoom    = errors.New("the server isn't currently in the room")
)

// CreateInvitesFrom3PIDInvites implements POST /_matrix/federation/v1/3pid/onbind
func CreateInvitesFrom3PIDInvites(
	req *http.Request, rsAPI api.RoomserverInternalAPI,
	cfg *config.FederationAPI,
	federation *gomatrixserverlib.FederationClient,
	userAPI userapi.UserInternalAPI,
) util.JSONResponse {
	var body invites
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	evs := []*gomatrixserverlib.HeaderedEvent{}
	for _, inv := range body.Invites {
		verReq := api.QueryRoomVersionForRoomRequest{RoomID: inv.RoomID}
		verRes := api.QueryRoomVersionForRoomResponse{}
		if err := rsAPI.QueryRoomVersionForRoom(req.Context(), &verReq, &verRes); err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.UnsupportedRoomVersion(err.Error()),
			}
		}

		event, err := createInviteFrom3PIDInvite(
			req.Context(), rsAPI, cfg, inv, federation, userAPI,
		)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("createInviteFrom3PIDInvite failed")
			return jsonerror.InternalServerError()
		}
		if event != nil {
			evs = append(evs, event.Headered(verRes.RoomVersion))
		}
	}

	// Send all the events
	if err := api.SendEvents(req.Context(), rsAPI, api.KindNew, evs, cfg.Matrix.ServerName, nil, false); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("SendEvents failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// ExchangeThirdPartyInvite implements PUT /_matrix/federation/v1/exchange_third_party_invite/{roomID}
func ExchangeThirdPartyInvite(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	roomID string,
	rsAPI api.RoomserverInternalAPI,
	cfg *config.FederationAPI,
	federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	var builder gomatrixserverlib.EventBuilder
	if err := json.Unmarshal(request.Content(), &builder); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	// Check that the room ID is correct.
	if builder.RoomID != roomID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The room ID in the request path must match the room ID in the invite event JSON"),
		}
	}

	// Check that the state key is correct.
	_, targetDomain, err := gomatrixserverlib.SplitID('@', *builder.StateKey)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event's state key isn't a Matrix user ID"),
		}
	}

	// Check that the target user is from the requesting homeserver.
	if targetDomain != request.Origin() {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event's state key doesn't have the same domain as the request's origin"),
		}
	}

	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err = rsAPI.QueryRoomVersionForRoom(httpReq.Context(), &verReq, &verRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion(err.Error()),
		}
	}

	// Auth and build the event from what the remote server sent us
	event, err := buildMembershipEvent(httpReq.Context(), &builder, rsAPI, cfg)
	if err == errNotInRoom {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Unknown room " + roomID),
		}
	} else if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("buildMembershipEvent failed")
		return jsonerror.InternalServerError()
	}

	// Ask the requesting server to sign the newly created event so we know it
	// acknowledged it
	signedEvent, err := federation.SendInvite(httpReq.Context(), request.Origin(), event)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("federation.SendInvite failed")
		return jsonerror.InternalServerError()
	}

	// Send the event to the roomserver
	if err = api.SendEvents(
		httpReq.Context(), rsAPI,
		api.KindNew,
		[]*gomatrixserverlib.HeaderedEvent{
			signedEvent.Event.Headered(verRes.RoomVersion),
		},
		cfg.Matrix.ServerName,
		nil,
		false,
	); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("SendEvents failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// createInviteFrom3PIDInvite processes an invite provided by the identity server
// and creates a m.room.member event (with "invite" membership) from it.
// Returns an error if there was a problem building the event or fetching the
// necessary data to do so.
func createInviteFrom3PIDInvite(
	ctx context.Context, rsAPI api.RoomserverInternalAPI,
	cfg *config.FederationAPI,
	inv invite, federation *gomatrixserverlib.FederationClient,
	userAPI userapi.UserInternalAPI,
) (*gomatrixserverlib.Event, error) {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: inv.RoomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := rsAPI.QueryRoomVersionForRoom(ctx, &verReq, &verRes); err != nil {
		return nil, err
	}

	_, server, err := gomatrixserverlib.SplitID('@', inv.MXID)
	if err != nil {
		return nil, err
	}

	if server != cfg.Matrix.ServerName {
		return nil, errNotLocalUser
	}

	// Build the event
	builder := &gomatrixserverlib.EventBuilder{
		Type:     "m.room.member",
		Sender:   inv.Sender,
		RoomID:   inv.RoomID,
		StateKey: &inv.MXID,
	}

	var res userapi.QueryProfileResponse
	err = userAPI.QueryProfile(ctx, &userapi.QueryProfileRequest{
		UserID: inv.MXID,
	}, &res)
	if err != nil {
		return nil, err
	}

	content := gomatrixserverlib.MemberContent{
		AvatarURL:   res.AvatarURL,
		DisplayName: res.DisplayName,
		Membership:  gomatrixserverlib.Invite,
		ThirdPartyInvite: &gomatrixserverlib.MemberThirdPartyInvite{
			Signed: inv.Signed,
		},
	}

	if err = builder.SetContent(content); err != nil {
		return nil, err
	}

	event, err := buildMembershipEvent(ctx, builder, rsAPI, cfg)
	if err == errNotInRoom {
		return nil, sendToRemoteServer(ctx, inv, federation, cfg, *builder)
	}
	if err != nil {
		return nil, err
	}

	return event, nil
}

// buildMembershipEvent uses a builder for a m.room.member invite event derived
// from a third-party invite to auth and build the said event. Returns the said
// event.
// Returns errNotInRoom if the server is not in the room the invite is for.
// Returns an error if something failed during the process.
func buildMembershipEvent(
	ctx context.Context,
	builder *gomatrixserverlib.EventBuilder, rsAPI api.RoomserverInternalAPI,
	cfg *config.FederationAPI,
) (*gomatrixserverlib.Event, error) {
	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return nil, err
	}

	if len(eventsNeeded.Tuples()) == 0 {
		return nil, errors.New("expecting state tuples for event builder, got none")
	}

	// Ask the roomserver for information about this room
	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID:       builder.RoomID,
		StateToFetch: eventsNeeded.Tuples(),
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	if err = rsAPI.QueryLatestEventsAndState(ctx, &queryReq, &queryRes); err != nil {
		return nil, err
	}

	if !queryRes.RoomExists {
		// Use federation to auth the event
		return nil, errNotInRoom
	}

	// Auth the event locally
	builder.Depth = queryRes.Depth
	builder.PrevEvents = queryRes.LatestEvents

	authEvents := gomatrixserverlib.NewAuthEvents(nil)

	for i := range queryRes.StateEvents {
		err = authEvents.AddEvent(queryRes.StateEvents[i].Event)
		if err != nil {
			return nil, err
		}
	}

	if err = fillDisplayName(builder, authEvents); err != nil {
		return nil, err
	}

	refs, err := eventsNeeded.AuthEventReferences(&authEvents)
	if err != nil {
		return nil, err
	}
	builder.AuthEvents = refs

	event, err := builder.Build(
		time.Now(), cfg.Matrix.ServerName, cfg.Matrix.KeyID,
		cfg.Matrix.PrivateKey, queryRes.RoomVersion,
	)

	return event, err
}

// sendToRemoteServer uses federation to send an invite provided by an identity
// server to a remote server in case the current server isn't in the room the
// invite is for.
// Returns an error if it couldn't get the server names to reach or if all of
// them responded with an error.
func sendToRemoteServer(
	ctx context.Context, inv invite,
	federation *gomatrixserverlib.FederationClient, _ *config.FederationAPI,
	builder gomatrixserverlib.EventBuilder,
) (err error) {
	remoteServers := make([]gomatrixserverlib.ServerName, 2)
	_, remoteServers[0], err = gomatrixserverlib.SplitID('@', inv.Sender)
	if err != nil {
		return
	}
	// Fallback to the room's server if the sender's domain is the same as
	// the current server's
	_, remoteServers[1], err = gomatrixserverlib.SplitID('!', inv.RoomID)
	if err != nil {
		return
	}

	for _, server := range remoteServers {
		err = federation.ExchangeThirdPartyInvite(ctx, server, builder)
		if err == nil {
			return
		}
		logrus.WithError(err).Warnf("failed to send 3PID invite via %s", server)
	}

	return errors.New("failed to send 3PID invite via any server")
}

// fillDisplayName looks in a list of auth events for a m.room.third_party_invite
// event with the state key matching a given m.room.member event's content's token.
// If such an event is found, fills the "display_name" attribute of the
// "third_party_invite" structure in the m.room.member event with the display_name
// from the m.room.third_party_invite event.
// Returns an error if there was a problem parsing the m.room.third_party_invite
// event's content or updating the m.room.member event's content.
// Returns nil if no m.room.third_party_invite with a matching token could be
// found. Returning an error isn't necessary in this case as the event will be
// rejected by gomatrixserverlib.
func fillDisplayName(
	builder *gomatrixserverlib.EventBuilder, authEvents gomatrixserverlib.AuthEvents,
) error {
	var content gomatrixserverlib.MemberContent
	if err := json.Unmarshal(builder.Content, &content); err != nil {
		return err
	}

	// Look for the m.room.third_party_invite event
	thirdPartyInviteEvent, _ := authEvents.ThirdPartyInvite(content.ThirdPartyInvite.Signed.Token)

	if thirdPartyInviteEvent == nil {
		// If the third party invite event doesn't exist then we can't use it to set the display name.
		return nil
	}

	var thirdPartyInviteContent gomatrixserverlib.ThirdPartyInviteContent
	if err := json.Unmarshal(thirdPartyInviteEvent.Content(), &thirdPartyInviteContent); err != nil {
		return err
	}

	// Use the m.room.third_party_invite event to fill the "displayname" and
	// update the m.room.member event's content with it
	content.ThirdPartyInvite.DisplayName = thirdPartyInviteContent.DisplayName
	return builder.SetContent(content)
}
