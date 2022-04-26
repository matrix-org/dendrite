// Copyright 2020 New Vector Ltd
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
	"net/http"

	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func PeekRoomByIDOrAlias(
	req *http.Request,
	device *api.Device,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	roomIDOrAlias string,
) util.JSONResponse {
	// if this is a remote roomIDOrAlias, we have to ask the roomserver (or federation sender?) to
	// to call /peek and /state on the remote server.
	// TODO: in future we could skip this if we know we're already participating in the room,
	// but this is fiddly in case we stop participating in the room.

	// then we create a local peek.
	peekReq := roomserverAPI.PerformPeekRequest{
		RoomIDOrAlias: roomIDOrAlias,
		UserID:        device.UserID,
		DeviceID:      device.ID,
	}
	peekRes := roomserverAPI.PerformPeekResponse{}

	// Check to see if any ?server_name= query parameters were
	// given in the request.
	if serverNames, ok := req.URL.Query()["server_name"]; ok {
		for _, serverName := range serverNames {
			peekReq.ServerNames = append(
				peekReq.ServerNames,
				gomatrixserverlib.ServerName(serverName),
			)
		}
	}

	// Ask the roomserver to perform the peek.
	rsAPI.PerformPeek(req.Context(), &peekReq, &peekRes)
	if peekRes.Error != nil {
		return peekRes.Error.JSONResponse()
	}

	// if this user is already joined to the room, we let them peek anyway
	// (given they might be about to part the room, and it makes things less fiddly)

	// Peeking stops if none of the devices who started peeking have been
	// /syncing for a while, or if everyone who was peeking calls /leave
	// (or /unpeek with a server_name param? or DELETE /peek?)
	// on the peeked room.

	return util.JSONResponse{
		Code: http.StatusOK,
		// TODO: Put the response struct somewhere internal.
		JSON: struct {
			RoomID string `json:"room_id"`
		}{peekRes.RoomID},
	}
}

func UnpeekRoomByID(
	req *http.Request,
	device *api.Device,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	roomID string,
) util.JSONResponse {
	unpeekReq := roomserverAPI.PerformUnpeekRequest{
		RoomID:   roomID,
		UserID:   device.UserID,
		DeviceID: device.ID,
	}
	unpeekRes := roomserverAPI.PerformUnpeekResponse{}

	rsAPI.PerformUnpeek(req.Context(), &unpeekReq, &unpeekRes)
	if unpeekRes.Error != nil {
		return unpeekRes.Error.JSONResponse()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
