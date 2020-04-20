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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// JoinRoomByIDOrAlias implements the "/join/{roomIDOrAlias}" API.
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-join-roomidoralias
func JoinRoomByIDOrAlias(
	req *http.Request,
	device *authtypes.Device,
	roomIDOrAlias string,
	cfg *config.Dendrite,
	federation *gomatrixserverlib.FederationClient,
	producer *producers.RoomserverProducer,
	queryAPI roomserverAPI.RoomserverQueryAPI,
	aliasAPI roomserverAPI.RoomserverAliasAPI,
	keyRing gomatrixserverlib.KeyRing,
	accountDB accounts.Database,
) util.JSONResponse {
	var content map[string]interface{} // must be a JSON object
	if resErr := httputil.UnmarshalJSONRequest(req, &content); resErr != nil {
		return *resErr
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	profile, err := accountDB.GetProfileByLocalpart(req.Context(), localpart)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("accountDB.GetProfileByLocalpart failed")
		return jsonerror.InternalServerError()
	}

	content["membership"] = gomatrixserverlib.Join
	content["displayname"] = profile.DisplayName
	content["avatar_url"] = profile.AvatarURL

	r := joinRoomReq{
		req, evTime, content, device.UserID, cfg, federation, producer, queryAPI, aliasAPI, keyRing,
	}

	if strings.HasPrefix(roomIDOrAlias, "!") {
		return r.joinRoomByID(roomIDOrAlias)
	}
	if strings.HasPrefix(roomIDOrAlias, "#") {
		return r.joinRoomByAlias(roomIDOrAlias)
	}
	return util.JSONResponse{
		Code: http.StatusBadRequest,
		JSON: jsonerror.BadJSON(
			fmt.Sprintf("Invalid first character '%s' for room ID or alias",
				string([]rune(roomIDOrAlias)[0])), // Wrapping with []rune makes this call UTF-8 safe
		),
	}
}

type joinRoomReq struct {
	req        *http.Request
	evTime     time.Time
	content    map[string]interface{}
	userID     string
	cfg        *config.Dendrite
	federation *gomatrixserverlib.FederationClient
	producer   *producers.RoomserverProducer
	queryAPI   roomserverAPI.RoomserverQueryAPI
	aliasAPI   roomserverAPI.RoomserverAliasAPI
	keyRing    gomatrixserverlib.KeyRing
}

// joinRoomByID joins a room by room ID
func (r joinRoomReq) joinRoomByID(roomID string) util.JSONResponse {
	// A client should only join a room by room ID when it has an invite
	// to the room. If the server is already in the room then we can
	// lookup the invite and process the request as a normal state event.
	// If the server is not in the room the we will need to look up the
	// remote server the invite came from in order to request a join event
	// from that server.
	queryReq := roomserverAPI.QueryInvitesForUserRequest{
		RoomID: roomID, TargetUserID: r.userID,
	}
	var queryRes roomserverAPI.QueryInvitesForUserResponse
	if err := r.queryAPI.QueryInvitesForUser(r.req.Context(), &queryReq, &queryRes); err != nil {
		util.GetLogger(r.req.Context()).WithError(err).Error("r.queryAPI.QueryInvitesForUser failed")
		return jsonerror.InternalServerError()
	}

	servers := []gomatrixserverlib.ServerName{}
	seenInInviterIDs := map[gomatrixserverlib.ServerName]bool{}
	for _, userID := range queryRes.InviteSenderUserIDs {
		_, domain, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil {
			util.GetLogger(r.req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
			return jsonerror.InternalServerError()
		}
		if !seenInInviterIDs[domain] {
			servers = append(servers, domain)
			seenInInviterIDs[domain] = true
		}
	}

	// Also add the domain extracted from the roomID as a last resort to join
	// in case the client is erroneously trying to join by ID without an invite
	// or all previous attempts at domains extracted from the inviter IDs fail
	// Note: It's no guarantee we'll succeed because a room isn't bound to the domain in its ID
	_, domain, err := gomatrixserverlib.SplitID('!', roomID)
	if err != nil {
		util.GetLogger(r.req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}
	if domain != r.cfg.Matrix.ServerName && !seenInInviterIDs[domain] {
		servers = append(servers, domain)
	}

	return r.joinRoomUsingServers(roomID, servers)

}

// joinRoomByAlias joins a room using a room alias.
func (r joinRoomReq) joinRoomByAlias(roomAlias string) util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('#', roomAlias)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}
	if domain == r.cfg.Matrix.ServerName {
		queryReq := roomserverAPI.GetRoomIDForAliasRequest{Alias: roomAlias}
		var queryRes roomserverAPI.GetRoomIDForAliasResponse
		if err = r.aliasAPI.GetRoomIDForAlias(r.req.Context(), &queryReq, &queryRes); err != nil {
			util.GetLogger(r.req.Context()).WithError(err).Error("r.aliasAPI.GetRoomIDForAlias failed")
			return jsonerror.InternalServerError()
		}

		if len(queryRes.RoomID) > 0 {
			return r.joinRoomUsingServers(queryRes.RoomID, []gomatrixserverlib.ServerName{r.cfg.Matrix.ServerName})
		}
		// If the response doesn't contain a non-empty string, return an error
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room alias " + roomAlias + " not found."),
		}
	}
	// If the room isn't local, use federation to join
	return r.joinRoomByRemoteAlias(domain, roomAlias)
}

func (r joinRoomReq) joinRoomByRemoteAlias(
	domain gomatrixserverlib.ServerName, roomAlias string,
) util.JSONResponse {
	resp, err := r.federation.LookupRoomAlias(r.req.Context(), domain, roomAlias)
	if err != nil {
		switch x := err.(type) {
		case gomatrix.HTTPError:
			if x.Code == http.StatusNotFound {
				return util.JSONResponse{
					Code: http.StatusNotFound,
					JSON: jsonerror.NotFound("Room alias not found"),
				}
			}
		}
		util.GetLogger(r.req.Context()).WithError(err).Error("r.federation.LookupRoomAlias failed")
		return jsonerror.InternalServerError()
	}

	return r.joinRoomUsingServers(resp.RoomID, resp.Servers)
}

func (r joinRoomReq) writeToBuilder(eb *gomatrixserverlib.EventBuilder, roomID string) error {
	eb.Type = "m.room.member"

	err := eb.SetContent(r.content)
	if err != nil {
		return err
	}

	err = eb.SetUnsigned(struct{}{})
	if err != nil {
		return err
	}

	eb.Sender = r.userID
	eb.StateKey = &r.userID
	eb.RoomID = roomID
	eb.Redacts = ""

	return nil
}

func (r joinRoomReq) joinRoomUsingServers(
	roomID string, servers []gomatrixserverlib.ServerName,
) util.JSONResponse {
	var eb gomatrixserverlib.EventBuilder
	err := r.writeToBuilder(&eb, roomID)
	if err != nil {
		util.GetLogger(r.req.Context()).WithError(err).Error("r.writeToBuilder failed")
		return jsonerror.InternalServerError()
	}

	queryRes := roomserverAPI.QueryLatestEventsAndStateResponse{}
	event, err := common.BuildEvent(r.req.Context(), &eb, r.cfg, r.evTime, r.queryAPI, &queryRes)
	if err == nil {
		// If we have successfully built an event at this point then we can
		// assert that the room is a local room, as BuildEvent was able to
		// add prev_events etc successfully.
		if _, err = r.producer.SendEvents(
			r.req.Context(),
			[]gomatrixserverlib.HeaderedEvent{
				(*event).Headered(queryRes.RoomVersion),
			},
			r.cfg.Matrix.ServerName,
			nil,
		); err != nil {
			util.GetLogger(r.req.Context()).WithError(err).Error("r.producer.SendEvents failed")
			return jsonerror.InternalServerError()
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct {
				RoomID string `json:"room_id"`
			}{roomID},
		}
	}

	// Otherwise, if we've reached here, then we haven't been able to populate
	// prev_events etc for the room, therefore the room is probably federated.

	// TODO: This needs to be re-thought, as in the case of an invite, the room
	// will exist in the database in roomserver_rooms but won't have any state
	// events, therefore this below check fails.
	if err != common.ErrRoomNoExists {
		util.GetLogger(r.req.Context()).WithError(err).Error("common.BuildEvent failed")
		return jsonerror.InternalServerError()
	}

	if len(servers) == 0 {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("No candidate servers found for room"),
		}
	}

	var lastErr error
	for _, server := range servers {
		var response *util.JSONResponse
		response, lastErr = r.joinRoomUsingServer(roomID, server)
		if lastErr != nil {
			// There was a problem talking to one of the servers.
			util.GetLogger(r.req.Context()).WithError(lastErr).WithField("server", server).Warn("Failed to join room using server")
			// Try the next server.
			if r.req.Context().Err() != nil {
				// The request context has expired so don't bother trying any
				// more servers - they will immediately fail due to the expired
				// context.
				break
			} else {
				// The request context hasn't expired yet so try the next server.
				continue
			}
		}
		return *response
	}

	// Every server we tried to join through resulted in an error.
	// We return the error from the last server.

	// TODO: Generate the correct HTTP status code for all different
	// kinds of errors that could have happened.
	// The possible errors include:
	//   1) We can't connect to the remote servers.
	//   2) None of the servers we could connect to think we are allowed
	//	    to join the room.
	//   3) The remote server returned something invalid.
	//   4) We couldn't fetch the public keys needed to verify the
	//      signatures on the state events.
	//   5) ...
	util.GetLogger(r.req.Context()).WithError(lastErr).Error("failed to join through any server")
	return jsonerror.InternalServerError()
}

// joinRoomUsingServer tries to join a remote room using a given matrix server.
// If there was a failure communicating with the server or the response from the
// server was invalid this returns an error.
// Otherwise this returns a JSONResponse.
func (r joinRoomReq) joinRoomUsingServer(roomID string, server gomatrixserverlib.ServerName) (*util.JSONResponse, error) {
	// Ask the room server for information about room versions.
	var request api.QueryRoomVersionCapabilitiesRequest
	var response api.QueryRoomVersionCapabilitiesResponse
	if err := r.queryAPI.QueryRoomVersionCapabilities(r.req.Context(), &request, &response); err != nil {
		return nil, err
	}
	var supportedVersions []gomatrixserverlib.RoomVersion
	for version := range response.AvailableRoomVersions {
		supportedVersions = append(supportedVersions, version)
	}
	respMakeJoin, err := r.federation.MakeJoin(r.req.Context(), server, roomID, r.userID, supportedVersions)
	if err != nil {
		// TODO: Check if the user was not allowed to join the room.
		return nil, fmt.Errorf("r.federation.MakeJoin: %w", err)
	}

	// Set all the fields to be what they should be, this should be a no-op
	// but it's possible that the remote server returned us something "odd"
	err = r.writeToBuilder(&respMakeJoin.JoinEvent, roomID)
	if err != nil {
		return nil, fmt.Errorf("r.writeToBuilder: %w", err)
	}

	if respMakeJoin.RoomVersion == "" {
		respMakeJoin.RoomVersion = gomatrixserverlib.RoomVersionV1
	}
	if _, err = respMakeJoin.RoomVersion.EventFormat(); err != nil {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion(
				fmt.Sprintf("Room version '%s' is not supported", respMakeJoin.RoomVersion),
			),
		}, nil
	}

	event, err := respMakeJoin.JoinEvent.Build(
		r.evTime, r.cfg.Matrix.ServerName, r.cfg.Matrix.KeyID,
		r.cfg.Matrix.PrivateKey, respMakeJoin.RoomVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("respMakeJoin.JoinEvent.Build: %w", err)
	}

	respSendJoin, err := r.federation.SendJoin(r.req.Context(), server, event, respMakeJoin.RoomVersion)
	if err != nil {
		return nil, fmt.Errorf("r.federation.SendJoin: %w", err)
	}

	if err = r.checkSendJoinResponse(event, server, respMakeJoin, respSendJoin); err != nil {
		return nil, err
	}

	util.GetLogger(r.req.Context()).WithFields(logrus.Fields{
		"room_id":          roomID,
		"num_auth_events":  len(respSendJoin.AuthEvents),
		"num_state_events": len(respSendJoin.StateEvents),
	}).Info("Room join signature and auth verification passed")

	// By this point we've verified all of the signatures, retrieved all of the
	// missing auth events and verified that everything checks out. Nothing
	// *should* go wrong in the roomserver after this point, so rather than have
	// the client block on the roomserver taking in all of the new events, we
	// should be okay to do this in a goroutine and return the successful join
	// back to the client.
	// TODO: Verify that this is really the case.
	go func() {
		ctx := context.Background()
		if err = r.producer.SendEventWithState(
			ctx,
			gomatrixserverlib.RespState(respSendJoin.RespState),
			event.Headered(respMakeJoin.RoomVersion),
		); err != nil {
			util.GetLogger(ctx).WithError(err).Error("r.producer.SendEventWithState")
		}
	}()

	return &util.JSONResponse{
		Code: http.StatusOK,
		// TODO: Put the response struct somewhere common.
		JSON: struct {
			RoomID string `json:"room_id"`
		}{roomID},
	}, nil
}

// checkSendJoinResponse checks that all of the signatures are correct
// and that the join is allowed by the supplied state.
func (r joinRoomReq) checkSendJoinResponse(
	event gomatrixserverlib.Event,
	server gomatrixserverlib.ServerName,
	respMakeJoin gomatrixserverlib.RespMakeJoin,
	respSendJoin gomatrixserverlib.RespSendJoin,
) error {
	// A list of events that we have retried, if they were not included in
	// the auth events supplied in the send_join.
	retries := map[string]bool{}

retryCheck:
	// TODO: Can we expand Check here to return a list of missing auth
	// events rather than failing one at a time?
	if err := respSendJoin.Check(r.req.Context(), r.keyRing, event); err != nil {
		switch e := err.(type) {
		case gomatrixserverlib.MissingAuthEventError:
			// Check that we haven't already retried for this event, prevents
			// us from ending up in endless loops
			if !retries[e.AuthEventID] {
				// Ask the server that we're talking to right now for the event
				tx, txerr := r.federation.GetEvent(r.req.Context(), server, e.AuthEventID)
				if txerr != nil {
					return fmt.Errorf("r.federation.GetEvent: %w", txerr)
				}
				// For each event returned, add it to the auth events.
				for _, pdu := range tx.PDUs {
					ev, everr := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, respMakeJoin.RoomVersion)
					if everr != nil {
						return fmt.Errorf("gomatrixserverlib.NewEventFromUntrustedJSON: %w", everr)
					}
					respSendJoin.AuthEvents = append(respSendJoin.AuthEvents, ev)
				}
				// Mark the event as retried and then give the check another go.
				retries[e.AuthEventID] = true
				goto retryCheck
			}
			return fmt.Errorf("respSendJoin (after retries): %w", e)
		default:
			return fmt.Errorf("respSendJoin: %w", err)
		}
	}
	return nil
}
