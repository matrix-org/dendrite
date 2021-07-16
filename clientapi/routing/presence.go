// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/eduserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type presenceRequest struct {
	Presence  string `json:"presence"`
	StatusMsg string `json:"status_msg"`
}

// The new presence state. One of: ["online", "offline", "unavailable"]
var allowedPresence = map[string]bool{
	"online":      true,
	"offline":     true,
	"unavailable": true,
}
var allowedStrings = make([]string, len(allowedPresence))

// we only need to do this once
func init() {
	i := 0
	for k := range allowedPresence {
		allowedStrings[i] = k
		i++
	}
}

func SetPresence(req *http.Request, eduAPI api.EDUServerInputAPI, device *userapi.Device) util.JSONResponse {
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be read: " + err.Error()),
		}
	}
	defer req.Body.Close() // nolint:errcheck

	// parse the request
	var r presenceRequest
	err = json.Unmarshal(data, &r)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be read: " + err.Error()),
		}
	}

	// requested new presence is not allowed by the spec
	if !allowedPresence[r.Presence] {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(
				fmt.Sprintf("The 'presence' field only allows one of: %s [sent: %s]", strings.Join(allowedStrings, ", "), r.Presence),
			),
		}
	}

	logrus.WithFields(logrus.Fields{
		"userId":     device.UserID,
		"presence":   r.Presence,
		"status_msg": r.StatusMsg,
	}).Debug("Setting presence for user")

	if err := api.SetPresence(
		req.Context(),
		eduAPI,
		device.ID,
		r.Presence,
		r.StatusMsg,
		gomatrixserverlib.AsTimestamp(time.Now()),
	); err != nil {
		return util.ErrorResponse(err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func GetPresence(req *http.Request, eduAPI api.EDUServerInputAPI, userID string) util.JSONResponse {

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
