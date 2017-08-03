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
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetMemberships implements GET /rooms/{roomId}/members
func GetMemberships(
	req *http.Request, device *authtypes.Device, roomID string,
	accountDB *accounts.Database, cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
) util.JSONResponse {
	localpart, server, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	events := []gomatrixserverlib.ClientEvent{}
	if server == cfg.Matrix.ServerName {
		// TODO: If the user has been in the room before but isn't
		// anymore, only send the members list as it was before they left.
		membership, err := accountDB.GetMembership(localpart, roomID)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if membership == nil {
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
			}
		}

		memberships, err := accountDB.GetMembershipsByRoomID(roomID)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		eventIDs := []string{}
		for _, membership := range memberships {
			eventIDs = append(eventIDs, membership.EventID)
		}

		queryReq := api.QueryEventsByIDRequest{
			EventIDs: eventIDs,
		}
		var queryRes api.QueryEventsByIDResponse
		if err := queryAPI.QueryEventsByID(&queryReq, &queryRes); err != nil {
			return httputil.LogThenError(req, err)
		}

		for _, event := range queryRes.Events {
			ev := gomatrixserverlib.ToClientEvent(event, gomatrixserverlib.FormatAll)
			events = append(events, ev)
		}
	} else {
		// TODO: Get memberships from federation
	}

	return util.JSONResponse{
		Code: 200,
		JSON: events,
	}
}
