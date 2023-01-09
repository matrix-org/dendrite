// Copyright 2017 New Vector Ltd
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
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RoomAliasToID converts the queried alias into a room ID and returns it
func RoomAliasToID(
	httpReq *http.Request,
	federation federationAPI.FederationClient,
	cfg *config.FederationAPI,
	rsAPI roomserverAPI.FederationRoomserverAPI,
	senderAPI federationAPI.FederationInternalAPI,
) util.JSONResponse {
	roomAlias := httpReq.FormValue("room_alias")
	if roomAlias == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Must supply room alias parameter."),
		}
	}
	_, domain, err := gomatrixserverlib.SplitID('#', roomAlias)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}

	var resp gomatrixserverlib.RespDirectory

	if domain == cfg.Matrix.ServerName {
		queryReq := &roomserverAPI.GetRoomIDForAliasRequest{
			Alias:              roomAlias,
			IncludeAppservices: true,
		}
		queryRes := &roomserverAPI.GetRoomIDForAliasResponse{}
		if err = rsAPI.GetRoomIDForAlias(httpReq.Context(), queryReq, queryRes); err != nil {
			util.GetLogger(httpReq.Context()).WithError(err).Error("aliasAPI.GetRoomIDForAlias failed")
			return jsonerror.InternalServerError()
		}

		if queryRes.RoomID != "" {
			serverQueryReq := federationAPI.QueryJoinedHostServerNamesInRoomRequest{RoomID: queryRes.RoomID}
			var serverQueryRes federationAPI.QueryJoinedHostServerNamesInRoomResponse
			if err = senderAPI.QueryJoinedHostServerNamesInRoom(httpReq.Context(), &serverQueryReq, &serverQueryRes); err != nil {
				util.GetLogger(httpReq.Context()).WithError(err).Error("senderAPI.QueryJoinedHostServerNamesInRoom failed")
				return jsonerror.InternalServerError()
			}

			resp = gomatrixserverlib.RespDirectory{
				RoomID:  queryRes.RoomID,
				Servers: serverQueryRes.ServerNames,
			}
		} else {
			// If no alias was found, return an error
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound(fmt.Sprintf("Room alias %s not found", roomAlias)),
			}
		}
	} else {
		resp, err = federation.LookupRoomAlias(httpReq.Context(), domain, cfg.Matrix.ServerName, roomAlias)
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
			// TODO: Return 502 if the remote server errored.
			// TODO: Return 504 if the remote server timed out.
			util.GetLogger(httpReq.Context()).WithError(err).Error("federation.LookupRoomAlias failed")
			return jsonerror.InternalServerError()
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}
