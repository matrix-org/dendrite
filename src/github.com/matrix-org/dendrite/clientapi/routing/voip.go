// Copyright 2017 Michael Telatysnki <7t3chguy@gmail.com>
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

	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/util"
)

type turnServerResponse struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	URIs     []string `json:"uris"`
	TTL      int      `json:"ttl"`
}

// RequestTurnServer implements:
//     GET /voip/turnServer
func RequestTurnServer(req *http.Request, device *authtypes.Device, cfg config.Dendrite) util.JSONResponse {
	turnConfig := cfg.TURN

	// TODO Guest Support
	if len(turnConfig.URIs) == 0 || turnConfig.UserLifetime == "" {
		return util.JSONResponse{
			Code: 200,
			JSON: struct{}{},
		}
	}

	// Duration checked at startup, err not possible
	duration, _ := time.ParseDuration(turnConfig.UserLifetime)

	resp := turnServerResponse{
		URIs: turnConfig.URIs,
		TTL:  int(duration.Seconds()),
	}

	if turnConfig.SharedSecret != "" {
		expiry := time.Now().Add(duration).Unix()
		mac := hmac.New(sha1.New, []byte(turnConfig.SharedSecret))
		_, err := mac.Write([]byte(resp.Username))

		if err != nil {
			return httputil.LogThenError(req, err)
		}

		resp.Username = fmt.Sprintf("%d:%s", expiry, device.UserID)
		resp.Password = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	} else if turnConfig.Username != "" && turnConfig.Password != "" {
		resp.Username = turnConfig.Username
		resp.Password = turnConfig.Password
	} else {
		return util.JSONResponse{
			Code: 200,
			JSON: struct{}{},
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: resp,
	}
}
