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

package writers

import (
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"net/http"
	"strings"
	"time"
)

// JoinRoomByIDOrAlias implements the "/join/{roomIDOrAlias}" API.
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-join-roomidoralias
func JoinRoomByIDOrAlias(
	req *http.Request,
	roomIDOrAlias string,
	cfg config.ClientAPI,
	federation *gomatrixserverlib.FederationClient,
	producer *producers.RoomserverProducer,
	queryAPI api.RoomserverQueryAPI,
) util.JSONResponse {
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}
	var content map[string]interface{} // must be a JSON object
	if resErr := httputil.UnmarshalJSONRequest(req, &content); resErr != nil {
		return *resErr
	}

	content["membership"] = "join"

	r := joinRoomReq{req, content, userID, cfg, federation, producer, queryAPI}

	if strings.HasPrefix(roomIDOrAlias, "!") {
		return r.joinRoomByID()
	}
	if strings.HasPrefix(roomIDOrAlias, "#") {
		return r.joinRoomByAlias(roomIDOrAlias)
	}
	return util.JSONResponse{
		Code: 400,
		JSON: jsonerror.BadJSON("Invalid first character for room ID or alias"),
	}
}

type joinRoomReq struct {
	req        *http.Request
	content    map[string]interface{}
	userID     string
	cfg        config.ClientAPI
	federation *gomatrixserverlib.FederationClient
	producer   *producers.RoomserverProducer
	queryAPI   api.RoomserverQueryAPI
}

// joinRoomByID joins a room by roomID
func (r joinRoomReq) joinRoomByID() util.JSONResponse {
	// TODO: Implement joining rooms by ID.
	// A client should only join a room by roomID when it has an invite
	// to the room. If the server is already in the room then we can
	// lookup the invite and process the request as a normal state event.
	// If the server is not in the room the we will need to look up the
	// remote server the invite came from in order to request a join event
	// from that server.
	panic(fmt.Errorf("Joining rooms by ID is not implemented"))
}

// joinRoomByAlias joins a room using a room alias.
func (r joinRoomReq) joinRoomByAlias(roomAlias string) util.JSONResponse {
	domain, err := domainFromID(roomAlias)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}
	if domain == r.cfg.ServerName {
		// TODO: Implement joining local room aliases.
		panic(fmt.Errorf("Joining local room aliases is not implemented"))
	} else {
		return r.joinRoomByRemoteAlias(domain, roomAlias)
	}
}

func (r joinRoomReq) joinRoomByRemoteAlias(
	domain gomatrixserverlib.ServerName, roomAlias string,
) util.JSONResponse {
	resp, err := r.federation.LookupRoomAlias(domain, roomAlias)
	if err != nil {
		switch x := err.(type) {
		case gomatrix.HTTPError:
			if x.Code == 404 {
				return util.JSONResponse{
					Code: 404,
					JSON: jsonerror.NotFound("Room alias not found"),
				}
			}
		}
		return util.ErrorResponse(err)
	}

	return r.joinRoomUsingServers(resp.RoomID, resp.Servers)
}

func (r joinRoomReq) writeToBuilder(eb *gomatrixserverlib.EventBuilder, roomID string) {
	eb.Type = "m.room.member"
	eb.SetContent(r.content) // TODO: Set avatar_url / displayname
	eb.SetUnsigned(struct{}{})
	eb.Sender = r.userID
	eb.StateKey = &r.userID
	eb.RoomID = roomID
	eb.Redacts = ""
}

func (r joinRoomReq) joinRoomUsingServers(
	roomID string, servers []gomatrixserverlib.ServerName,
) util.JSONResponse {
	var eb gomatrixserverlib.EventBuilder
	r.writeToBuilder(&eb, roomID)

	needed, err := gomatrixserverlib.StateNeededForEventBuilder(&eb)
	if err != nil {
		return httputil.LogThenError(r.req, err)
	}

	// Ask the roomserver for information about this room
	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID:       roomID,
		StateToFetch: needed.Tuples(),
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	if queryErr := r.queryAPI.QueryLatestEventsAndState(&queryReq, &queryRes); queryErr != nil {
		return httputil.LogThenError(r.req, queryErr)
	}

	if queryRes.RoomExists {
		// TODO: Implement joining rooms that already the server is already in.
		// This should just fall through to the usual event sending code.
		panic(fmt.Errorf("Joining rooms that the server already in is not implemented"))
	}

	var event gomatrixserverlib.Event
	var respMakeJoin gomatrixserverlib.RespMakeJoin
	//var respSendJoin gomatrixserverlib.RespSendJoin
	for _, server := range servers {
		respMakeJoin, err = r.federation.MakeJoin(server, roomID, r.userID)
		if err != nil {
			// TODO: Check if the user was not allowed to join the room.

			// Try the next server in the list.
			continue
		}

		// Set all the fields to be what they should be, this should be a no-op
		// but it's possible that the remote server returned us something "odd"
		r.writeToBuilder(&respMakeJoin.JoinEvent, roomID)

		now := time.Now()
		eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), r.cfg.ServerName)
		if event, err = respMakeJoin.JoinEvent.Build(
			eventID, now, r.cfg.ServerName, r.cfg.KeyID, r.cfg.PrivateKey,
		); err != nil {
			return util.ErrorResponse(err)
		}

		_, err = r.federation.SendJoin(server, event)
		if err != nil {
			// Try the next server in the list.
			continue
		}

		// TODO: validate the state response.
		// TODO: check that the join event passes auth against the state response.
		// TODO: send the state and the join event to the room server.

		return util.JSONResponse{
			Code: 200,
			// TODO: Put the response struct somewhere common.
			JSON: struct {
				RoomID string `json:"room_id"`
			}{roomID},
		}
	}

	if err != nil {
		return util.ErrorResponse(err)
	}

	return util.JSONResponse{
		Code: 404,
		JSON: jsonerror.NotFound("No candidate servers found for room"),
	}
}

// domainFromID returns everything after the first ":" character to extract
// the domain part of a matrix ID.
// TODO: duplicated from gomatrixserverlib.
func domainFromID(id string) (gomatrixserverlib.ServerName, error) {
	// IDs have the format: SIGIL LOCALPART ":" DOMAIN
	// Split on the first ":" character since the domain can contain ":"
	// characters.
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		// The ID must have a ":" character.
		return "", fmt.Errorf("invalid ID: %q", id)
	}
	// Return everything after the first ":" character.
	return gomatrixserverlib.ServerName(parts[1]), nil
}
