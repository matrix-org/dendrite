// Copyright 2024 New Vector Ltd.
// Copyright 2017 Michael Telatysnki <7t3chguy@gmail.com>
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/util"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// RequestTurnServer implements:
//
//	GET /voip/turnServer
func RequestTurnServer(req *http.Request, device *api.Device, cfg *config.ClientAPI) util.JSONResponse {
	turnConfig := cfg.TURN

	// TODO Guest Support
	if len(turnConfig.URIs) == 0 || turnConfig.UserLifetime == "" {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	// Duration checked at startup, err not possible
	duration, _ := time.ParseDuration(turnConfig.UserLifetime)

	resp := gomatrix.RespTurnServer{
		URIs: turnConfig.URIs,
		TTL:  int(duration.Seconds()),
	}

	if turnConfig.SharedSecret != "" {
		expiry := time.Now().Add(duration).Unix()
		resp.Username = fmt.Sprintf("%d:%s", expiry, device.UserID)
		mac := hmac.New(sha1.New, []byte(turnConfig.SharedSecret))
		_, err := mac.Write([]byte(resp.Username))

		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("mac.Write failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		resp.Password = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	} else if turnConfig.Username != "" && turnConfig.Password != "" {
		resp.Username = turnConfig.Username
		resp.Password = turnConfig.Password
	} else {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}
