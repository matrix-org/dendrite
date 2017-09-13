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

package readers

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// DirectoryRoom looks up a room alias
func DirectoryRoom(
	req *http.Request,
	roomAlias string,
	federation *gomatrixserverlib.FederationClient,
	cfg *config.Dendrite,
	aliasAPI api.RoomserverAliasAPI,
) util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('#', roomAlias)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}

	var resp gomatrixserverlib.RespDirectory

	if domain == cfg.Matrix.ServerName {
		queryReq := api.GetAliasRoomIDRequest{Alias: roomAlias}
		var queryRes api.GetAliasRoomIDResponse
		if err = aliasAPI.GetAliasRoomID(req.Context(), &queryReq, &queryRes); err != nil {
			return httputil.LogThenError(req, err)
		}

		if len(queryRes.RoomID) > 0 {
			// TODO: List servers that are aware of this room alias
			resp = gomatrixserverlib.RespDirectory{
				RoomID:  queryRes.RoomID,
				Servers: []gomatrixserverlib.ServerName{},
			}
		} else {
			// If the response doesn't contain a non-empty string, return an error
			return util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound("Room alias " + roomAlias + " not found."),
			}
		}
	} else {
		resp, err = federation.LookupRoomAlias(req.Context(), domain, roomAlias)
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
			// TODO: Return 502 if the remote server errored.
			// TODO: Return 504 if the remote server timed out.
			return httputil.LogThenError(req, err)
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: resp,
	}
}

// SetLocalAlias implements PUT /directory/room/{roomAlias}
// TODO: Check if the user has the power level to set an alias
func SetLocalAlias(
	req *http.Request,
	device *authtypes.Device,
	alias string,
	cfg *config.Dendrite,
	aliasAPI api.RoomserverAliasAPI,
) util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('#', alias)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}

	if domain != cfg.Matrix.ServerName {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("Alias must be on local homeserver"),
		}
	}

	var r struct {
		RoomID string `json:"room_id"`
	}
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}

	queryReq := api.SetRoomAliasRequest{
		UserID: device.UserID,
		RoomID: r.RoomID,
		Alias:  alias,
	}
	var queryRes api.SetRoomAliasResponse
	if err := aliasAPI.SetRoomAlias(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	if queryRes.AliasExists {
		return util.JSONResponse{
			Code: 409,
			JSON: jsonerror.Unknown("The alias " + alias + " already exists."),
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

// RemoveLocalAlias implements DELETE /directory/room/{roomAlias}
// TODO: Check if the user has the power level to remove an alias
func RemoveLocalAlias(
	req *http.Request,
	device *authtypes.Device,
	alias string,
	aliasAPI api.RoomserverAliasAPI,
) util.JSONResponse {
	queryReq := api.RemoveRoomAliasRequest{
		Alias:  alias,
		UserID: device.UserID,
	}
	var queryRes api.RemoveRoomAliasResponse
	if err := aliasAPI.RemoveRoomAlias(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}
