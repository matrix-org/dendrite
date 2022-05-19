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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
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
	userID string, rsAPI roomserverAPI.ClientRoomserverAPI,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {
	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot set another user's typing state"),
		}
	}

	// Verify that the user is a member of this room
	resErr := checkMemberInRoom(req.Context(), rsAPI, userID, roomID)
	if resErr != nil {
		return *resErr
	}

	// parse the incoming http request
	var r typingContentJSON
	resErr = httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	if err := syncProducer.SendTyping(req.Context(), userID, roomID, r.Typing, r.Timeout); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("eduProducer.Send failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
