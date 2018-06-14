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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RoomAliasToID converts the queried alias into a room ID and returns it
func RoomAliasToID(
	httpReq *http.Request,
	federation *gomatrixserverlib.FederationClient,
	cfg config.Dendrite,
	aliasAPI roomserverAPI.RoomserverAliasAPI,
) util.JSONResponse {
	roomAlias := httpReq.FormValue("alias")
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
		queryReq := roomserverAPI.GetRoomIDForAliasRequest{Alias: roomAlias}
		var queryRes roomserverAPI.GetRoomIDForAliasResponse
		if err = aliasAPI.GetRoomIDForAlias(httpReq.Context(), &queryReq, &queryRes); err != nil {
			return httputil.LogThenError(httpReq, err)
		}

		if queryRes.RoomID == "" {
			// TODO: List servers that are aware of this room alias
			resp = gomatrixserverlib.RespDirectory{
				RoomID:  queryRes.RoomID,
				Servers: []gomatrixserverlib.ServerName{},
			}
		} else {
			// If the response doesn't contain a non-empty string, return an error
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound(fmt.Sprintf("Room alias %s not found", roomAlias)),
			}
		}
	} else {
		resp, err = federation.LookupRoomAlias(httpReq.Context(), domain, roomAlias)
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
			return httputil.LogThenError(httpReq, err)
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}
