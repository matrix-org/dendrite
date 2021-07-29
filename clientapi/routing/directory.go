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

package routing

import (
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
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
	cfg *config.ClientAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	fedSenderAPI federationSenderAPI.FederationSenderInternalAPI,
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
	queryReq := &roomserverAPI.GetRoomIDForAliasRequest{
		Alias:              roomAlias,
		IncludeAppservices: true,
	}
	queryRes := &roomserverAPI.GetRoomIDForAliasResponse{}
	if err = rsAPI.GetRoomIDForAlias(req.Context(), queryReq, queryRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.GetRoomIDForAlias failed")
		return jsonerror.InternalServerError()
	}

	res.RoomID = queryRes.RoomID

	if res.RoomID == "" {
		// If we don't know it locally, do a federation query.
		// But don't send the query to ourselves.
		if domain != cfg.Matrix.ServerName {
			fedRes, fedErr := federation.LookupRoomAlias(req.Context(), domain, roomAlias)
			if fedErr != nil {
				// TODO: Return 502 if the remote server errored.
				// TODO: Return 504 if the remote server timed out.
				util.GetLogger(req.Context()).WithError(fedErr).Error("federation.LookupRoomAlias failed")
				return jsonerror.InternalServerError()
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
			util.GetLogger(req.Context()).WithError(err).Error("fedSenderAPI.QueryJoinedHostServerNamesInRoom failed")
			return jsonerror.InternalServerError()
		}
		res.fillServers(joinedHostsRes.ServerNames)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

// SetLocalAlias implements PUT /directory/room/{roomAlias}
func SetLocalAlias(
	req *http.Request,
	device *api.Device,
	alias string,
	cfg *config.ClientAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
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
	if err := rsAPI.SetRoomAlias(req.Context(), &queryReq, &queryRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("aliasAPI.SetRoomAlias failed")
		return jsonerror.InternalServerError()
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
	device *api.Device,
	alias string,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) util.JSONResponse {
	queryReq := roomserverAPI.RemoveRoomAliasRequest{
		Alias:  alias,
		UserID: device.UserID,
	}
	var queryRes roomserverAPI.RemoveRoomAliasResponse
	if err := rsAPI.RemoveRoomAlias(req.Context(), &queryReq, &queryRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("aliasAPI.RemoveRoomAlias failed")
		return jsonerror.InternalServerError()
	}

	if !queryRes.Found {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("The alias does not exist."),
		}
	}

	if !queryRes.Removed {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You do not have permission to remove this alias."),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

type roomVisibility struct {
	Visibility string `json:"visibility"`
}

// GetVisibility implements GET /directory/list/room/{roomID}
func GetVisibility(
	req *http.Request, rsAPI roomserverAPI.RoomserverInternalAPI,
	roomID string,
) util.JSONResponse {
	var res roomserverAPI.QueryPublishedRoomsResponse
	err := rsAPI.QueryPublishedRooms(req.Context(), &roomserverAPI.QueryPublishedRoomsRequest{
		RoomID: roomID,
	}, &res)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryPublishedRooms failed")
		return jsonerror.InternalServerError()
	}

	var v roomVisibility
	if len(res.RoomIDs) == 1 {
		v.Visibility = gomatrixserverlib.Public
	} else {
		v.Visibility = "private"
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: v,
	}
}

// SetVisibility implements PUT /directory/list/room/{roomID}
// TODO: Allow admin users to edit the room visibility
func SetVisibility(
	req *http.Request, rsAPI roomserverAPI.RoomserverInternalAPI, dev *userapi.Device,
	roomID string,
) util.JSONResponse {
	resErr := checkMemberInRoom(req.Context(), rsAPI, dev.UserID, roomID)
	if resErr != nil {
		return *resErr
	}

	queryEventsReq := roomserverAPI.QueryLatestEventsAndStateRequest{
		RoomID: roomID,
		StateToFetch: []gomatrixserverlib.StateKeyTuple{{
			EventType: gomatrixserverlib.MRoomPowerLevels,
			StateKey:  "",
		}},
	}
	var queryEventsRes roomserverAPI.QueryLatestEventsAndStateResponse
	err := rsAPI.QueryLatestEventsAndState(req.Context(), &queryEventsReq, &queryEventsRes)
	if err != nil || len(queryEventsRes.StateEvents) == 0 {
		util.GetLogger(req.Context()).WithError(err).Error("could not query events from room")
		return jsonerror.InternalServerError()
	}

	// NOTSPEC: Check if the user's power is greater than power required to change m.room.canonical_alias event
	power, _ := gomatrixserverlib.NewPowerLevelContentFromEvent(queryEventsRes.StateEvents[0].Event)
	if power.UserLevel(dev.UserID) < power.EventLevel(gomatrixserverlib.MRoomCanonicalAlias, true) {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID doesn't have power level to change visibility"),
		}
	}

	var v roomVisibility
	if reqErr := httputil.UnmarshalJSONRequest(req, &v); reqErr != nil {
		return *reqErr
	}

	var publishRes roomserverAPI.PerformPublishResponse
	rsAPI.PerformPublish(req.Context(), &roomserverAPI.PerformPublishRequest{
		RoomID:     roomID,
		Visibility: v.Visibility,
	}, &publishRes)
	if publishRes.Error != nil {
		util.GetLogger(req.Context()).WithError(publishRes.Error).Error("PerformPublish failed")
		return publishRes.Error.JSONResponse()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
