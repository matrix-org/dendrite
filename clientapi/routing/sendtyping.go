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
	"database/sql"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/eduserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type typingContentJSON struct {
	Typing  bool  `json:"typing"`
	Timeout int64 `json:"timeout"`
}

// SendTyping handles PUT /rooms/{roomID}/typing/{userID}
// sends the typing events to client API typingProducer
func SendTyping(
	req *http.Request, device *userapi.Device, roomID string,
	userID string, accountDB accounts.Database,
	eduAPI api.EDUServerInputAPI,
) util.JSONResponse {
	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot set another user's typing state"),
		}
	}

	localpart, err := userutil.ParseUsernameParam(userID, nil)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("userutil.ParseUsernameParam failed")
		return jsonerror.InternalServerError()
	}

	// Verify that the user is a member of this room
	_, err = accountDB.GetMembershipInRoomByLocalpart(req.Context(), localpart, roomID)
	if err == sql.ErrNoRows {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("User not in this room"),
		}
	} else if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("accountDB.GetMembershipInRoomByLocalPart failed")
		return jsonerror.InternalServerError()
	}

	// parse the incoming http request
	var r typingContentJSON
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	if err = api.SendTyping(
		req.Context(), eduAPI, userID, roomID, r.Typing, r.Timeout,
	); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("eduProducer.Send failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
