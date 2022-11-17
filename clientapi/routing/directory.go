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

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
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
	rsAPI roomserverAPI.ClientRoomserverAPI,
	fedSenderAPI federationAPI.ClientFederationAPI,
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
		if !cfg.Matrix.IsLocalServerName(domain) {
			fedRes, fedErr := federation.LookupRoomAlias(req.Context(), cfg.Matrix.ServerName, domain, roomAlias)
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
		joinedHostsReq := federationAPI.QueryJoinedHostServerNamesInRoomRequest{RoomID: res.RoomID}
		var joinedHostsRes federationAPI.QueryJoinedHostServerNamesInRoomResponse
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
	device *userapi.Device,
	alias string,
	cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI,
) util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('#', alias)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Room alias must be in the form '#localpart:domain'"),
		}
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
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
	reqUserID, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("User ID must be in the form '@localpart:domain'"),
		}
	}
	for _, appservice := range cfg.Derived.ApplicationServices {
		// Don't prevent AS from creating aliases in its own namespace
		// Note that Dendrite uses SenderLocalpart as UserID for AS users
		if reqUserID != appservice.SenderLocalpart {
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
	device *userapi.Device,
	alias string,
	rsAPI roomserverAPI.ClientRoomserverAPI,
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
	req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI,
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
	req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI, dev *userapi.Device,
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
	if err := rsAPI.PerformPublish(req.Context(), &roomserverAPI.PerformPublishRequest{
		RoomID:     roomID,
		Visibility: v.Visibility,
	}, &publishRes); err != nil {
		return jsonerror.InternalAPIError(req.Context(), err)
	}
	if publishRes.Error != nil {
		util.GetLogger(req.Context()).WithError(publishRes.Error).Error("PerformPublish failed")
		return publishRes.Error.JSONResponse()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func SetVisibilityAS(
	req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI, dev *userapi.Device,
	networkID, roomID string,
) util.JSONResponse {
	if dev.AccountType != userapi.AccountTypeAppService {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Only appservice may use this endpoint"),
		}
	}
	var v roomVisibility

	// If the method is delete, we simply mark the visibility as private
	if req.Method == http.MethodDelete {
		v.Visibility = "private"
	} else {
		if reqErr := httputil.UnmarshalJSONRequest(req, &v); reqErr != nil {
			return *reqErr
		}
	}
	var publishRes roomserverAPI.PerformPublishResponse
	if err := rsAPI.PerformPublish(req.Context(), &roomserverAPI.PerformPublishRequest{
		RoomID:       roomID,
		Visibility:   v.Visibility,
		NetworkID:    networkID,
		AppserviceID: dev.AppserviceID,
	}, &publishRes); err != nil {
		return jsonerror.InternalAPIError(req.Context(), err)
	}
	if publishRes.Error != nil {
		util.GetLogger(req.Context()).WithError(publishRes.Error).Error("PerformPublish failed")
		return publishRes.Error.JSONResponse()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
