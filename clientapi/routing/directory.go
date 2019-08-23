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
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type roomDirectoryResponse struct {
	RoomID  string   `json:"room_id"`
	Servers []string `json:"servers"`
}

func (r *roomDirectoryResponse) fillServers(servers []gomatrixserverlib.ServerName) {
	r.Servers = make([]string, len(servers))
	for i, s := range servers {
		r.Servers[i] = string(s)
	}
}

// DirectoryRoom looks up a room alias
func DirectoryRoom(
	req *http.Request,
	roomAlias string,
	federation *gomatrixserverlib.FederationClient,
	cfg *config.Dendrite,
	rsAPI roomserverAPI.RoomserverAliasAPI,
	fedSenderAPI federationSenderAPI.FederationSenderQueryAPI,
) util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('#', roomAlias)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}

	var res roomDirectoryResponse

	// Query the roomserver API to check if the alias exists locally.
	queryReq := roomserverAPI.GetRoomIDForAliasRequest{Alias: roomAlias}
	var queryRes roomserverAPI.GetRoomIDForAliasResponse
	if err = rsAPI.GetRoomIDForAlias(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	res.RoomID = queryRes.RoomID

	if res.RoomID == "" {
		// If we don't know it locally, do a federation query.
		// But don't send the query to ourselves.
		if domain != cfg.Matrix.ServerName {
			fedRes, fedErr := federation.LookupRoomAlias(req.Context(), domain, roomAlias)
			if fedErr != nil {
				switch fedErr.(type) {
				case gomatrix.HTTPError:
					// TODO: Return 502 if the remote server errored.
					// TODO: Return 504 if the remote server timed out.
					return httputil.LogThenError(req, fedErr)
				default:
					return httputil.LogThenError(req, fedErr)
				}
			}
			res.RoomID = fedRes.RoomID
			res.fillServers(fedRes.Servers)
		}

		if res.RoomID == "" {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound(
					fmt.Sprintf("Room alias %s not found", roomAlias),
				),
			}
		}
	} else {
		joinedHostsReq := federationSenderAPI.QueryJoinedHostServerNamesInRoomRequest{RoomID: res.RoomID}
		var joinedHostsRes federationSenderAPI.QueryJoinedHostServerNamesInRoomResponse
		if err = fedSenderAPI.QueryJoinedHostServerNamesInRoom(req.Context(), &joinedHostsReq, &joinedHostsRes); err != nil {
			return httputil.LogThenError(req, err)
		}
		res.fillServers(joinedHostsRes.ServerNames)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

// SetLocalAlias implements PUT /directory/room/{roomAlias}
// TODO: Check if the user has the power level to set an alias
func SetLocalAlias(
	req *http.Request,
	device *authtypes.Device,
	alias string,
	cfg *config.Dendrite,
	aliasAPI roomserverAPI.RoomserverAliasAPI,
) util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('#', alias)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}

	if domain != cfg.Matrix.ServerName {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Alias must be on local homeserver"),
		}
	}

	// Check that the alias does not fall within an exclusive namespace of an
	// application service
	// TODO: This code should eventually be refactored with:
	// 1. The new method for checking for things matching an AS's namespace
	// 2. Using an overall Regex object for all AS's just like we did for usernames
	for _, appservice := range cfg.Derived.ApplicationServices {
		// Don't prevent AS from creating aliases in its own namespace
		// Note that Dendrite uses SenderLocalpart as UserID for AS users
		if device.UserID != appservice.SenderLocalpart {
			if aliasNamespaces, ok := appservice.NamespaceMap["aliases"]; ok {
				for _, namespace := range aliasNamespaces {
					if namespace.Exclusive && namespace.RegexpObject.MatchString(alias) {
						return util.JSONResponse{
							Code: http.StatusBadRequest,
							JSON: jsonerror.ASExclusive("Alias is reserved by an application service"),
						}
					}
				}
			}
		}
	}

	var r struct {
		RoomID string `json:"room_id"`
	}
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}

	queryReq := roomserverAPI.SetRoomAliasRequest{
		UserID: device.UserID,
		RoomID: r.RoomID,
		Alias:  alias,
	}
	var queryRes roomserverAPI.SetRoomAliasResponse
	if err := aliasAPI.SetRoomAlias(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	if queryRes.AliasExists {
		return util.JSONResponse{
			Code: http.StatusConflict,
			JSON: jsonerror.Unknown("The alias " + alias + " already exists."),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// RemoveLocalAlias implements DELETE /directory/room/{roomAlias}
func RemoveLocalAlias(
	req *http.Request,
	device *authtypes.Device,
	alias string,
	aliasAPI roomserverAPI.RoomserverAliasAPI,
) util.JSONResponse {

	creatorQueryReq := roomserverAPI.GetCreatorIDForAliasRequest{
		Alias: alias,
	}
	var creatorQueryRes roomserverAPI.GetCreatorIDForAliasResponse
	if err := aliasAPI.GetCreatorIDForAlias(req.Context(), &creatorQueryReq, &creatorQueryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	if creatorQueryRes.UserID == "" {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Alias does not exist"),
		}
	}

	if creatorQueryRes.UserID != device.UserID {
		// TODO: Still allow deletion if user is admin
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You do not have permission to delete this alias"),
		}
	}

	queryReq := roomserverAPI.RemoveRoomAliasRequest{
		Alias:  alias,
		UserID: device.UserID,
	}
	var queryRes roomserverAPI.RemoveRoomAliasResponse
	if err := aliasAPI.RemoveRoomAlias(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
