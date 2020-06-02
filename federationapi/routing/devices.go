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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetUserDevices for the given user id
func GetUserDevices(
	req *http.Request,
	deviceDB devices.Database,
	userID string,
) util.JSONResponse {
	localpart, err := userutil.ParseUsernameParam(userID, nil)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("Invalid user ID"),
		}
	}

	response := gomatrixserverlib.RespUserDevices{
		UserID: userID,
		// TODO: we should return an incrementing stream ID each time the device
		// list changes for delta changes to be recognised
		StreamID: 0,
	}

	devs, err := deviceDB.GetDevicesByLocalpart(req.Context(), localpart)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("deviceDB.GetDevicesByLocalPart failed")
		return jsonerror.InternalServerError()
	}

	for _, dev := range devs {
		device := gomatrixserverlib.RespUserDevice{
			DeviceID:    dev.ID,
			DisplayName: dev.DisplayName,
			Keys:        []gomatrixserverlib.RespUserDeviceKeys{},
		}
		response.Devices = append(response.Devices, device)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}
