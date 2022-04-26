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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type crossSigningRequest struct {
	api.PerformUploadDeviceKeysRequest
	Auth newPasswordAuth `json:"auth"`
}

func UploadCrossSigningDeviceKeys(
	req *http.Request, userInteractiveAuth *auth.UserInteractive,
	keyserverAPI api.KeyInternalAPI, device *userapi.Device,
	accountAPI userapi.UserAccountAPI, cfg *config.ClientAPI,
) util.JSONResponse {
	uploadReq := &crossSigningRequest{}
	uploadRes := &api.PerformUploadDeviceKeysResponse{}

	resErr := httputil.UnmarshalJSONRequest(req, &uploadReq)
	if resErr != nil {
		return *resErr
	}
	sessionID := uploadReq.Auth.Session
	if sessionID == "" {
		sessionID = util.RandomString(sessionIDLength)
	}
	if uploadReq.Auth.Type != authtypes.LoginTypePassword {
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: newUserInteractiveResponse(
				sessionID,
				[]authtypes.Flow{
					{
						Stages: []authtypes.LoginType{authtypes.LoginTypePassword},
					},
				},
				nil,
			),
		}
	}
	typePassword := auth.LoginTypePassword{
		GetAccountByPassword: accountAPI.QueryAccountByPassword,
		Config:               cfg,
	}
	if _, authErr := typePassword.Login(req.Context(), &uploadReq.Auth.PasswordRequest); authErr != nil {
		return *authErr
	}
	sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypePassword)

	uploadReq.UserID = device.UserID
	keyserverAPI.PerformUploadDeviceKeys(req.Context(), &uploadReq.PerformUploadDeviceKeysRequest, uploadRes)

	if err := uploadRes.Error; err != nil {
		switch {
		case err.IsInvalidSignature:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidSignature(err.Error()),
			}
		case err.IsMissingParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.MissingParam(err.Error()),
			}
		case err.IsInvalidParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidParam(err.Error()),
			}
		default:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.Unknown(err.Error()),
			}
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func UploadCrossSigningDeviceSignatures(req *http.Request, keyserverAPI api.KeyInternalAPI, device *userapi.Device) util.JSONResponse {
	uploadReq := &api.PerformUploadDeviceSignaturesRequest{}
	uploadRes := &api.PerformUploadDeviceSignaturesResponse{}

	if err := httputil.UnmarshalJSONRequest(req, &uploadReq.Signatures); err != nil {
		return *err
	}

	uploadReq.UserID = device.UserID
	keyserverAPI.PerformUploadDeviceSignatures(req.Context(), uploadReq, uploadRes)

	if err := uploadRes.Error; err != nil {
		switch {
		case err.IsInvalidSignature:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidSignature(err.Error()),
			}
		case err.IsMissingParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.MissingParam(err.Error()),
			}
		case err.IsInvalidParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidParam(err.Error()),
			}
		default:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.Unknown(err.Error()),
			}
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
