// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"net/http"

	"github.com/element-hq/dendrite/clientapi/auth"
	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/clientapi/userutil"
	"github.com/element-hq/dendrite/setup/config"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type loginResponse struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
}

type flows struct {
	Flows []flow `json:"flows"`
}

type flow struct {
	Type string `json:"type"`
}

// Login implements GET and POST /login
func Login(
	req *http.Request, userAPI userapi.ClientUserAPI,
	cfg *config.ClientAPI,
) util.JSONResponse {
	if req.Method == http.MethodGet {
		loginFlows := []flow{{Type: authtypes.LoginTypePassword}}
		if len(cfg.Derived.ApplicationServices) > 0 {
			loginFlows = append(loginFlows, flow{Type: authtypes.LoginTypeApplicationService})
		}
		// TODO: support other forms of login, depending on config options
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: flows{
				Flows: loginFlows,
			},
		}
	} else if req.Method == http.MethodPost {
		login, cleanup, authErr := auth.LoginFromJSONReader(req, userAPI, userAPI, cfg)
		if authErr != nil {
			return *authErr
		}
		// make a device/access token
		authErr2 := completeAuth(req.Context(), cfg.Matrix, userAPI, login, req.RemoteAddr, req.UserAgent())
		cleanup(req.Context(), &authErr2)
		return authErr2
	}
	return util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: spec.NotFound("Bad method"),
	}
}

func completeAuth(
	ctx context.Context, cfg *config.Global, userAPI userapi.ClientUserAPI, login *auth.Login,
	ipAddr, userAgent string,
) util.JSONResponse {
	token, err := auth.GenerateAccessToken()
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("auth.GenerateAccessToken failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	localpart, serverName, err := userutil.ParseUsernameParam(login.Username(), cfg)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("auth.ParseUsernameParam failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	var performRes userapi.PerformDeviceCreationResponse
	err = userAPI.PerformDeviceCreation(ctx, &userapi.PerformDeviceCreationRequest{
		DeviceDisplayName: login.InitialDisplayName,
		DeviceID:          login.DeviceID,
		AccessToken:       token,
		Localpart:         localpart,
		ServerName:        serverName,
		IPAddr:            ipAddr,
		UserAgent:         userAgent,
	}, &performRes)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("failed to create device: " + err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: loginResponse{
			UserID:      performRes.Device.UserID,
			AccessToken: performRes.Device.AccessToken,
			DeviceID:    performRes.Device.ID,
		},
	}
}
