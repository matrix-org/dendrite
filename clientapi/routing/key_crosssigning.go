// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only
// Please see LICENSE in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/clientapi/auth"
	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type crossSigningRequest struct {
	api.PerformUploadDeviceKeysRequest
	Auth newPasswordAuth `json:"auth"`
}

func UploadCrossSigningDeviceKeys(
	req *http.Request, userInteractiveAuth *auth.UserInteractive,
	keyserverAPI api.ClientKeyAPI, device *api.Device,
	accountAPI api.ClientUserAPI, cfg *config.ClientAPI,
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
				JSON: spec.InvalidSignature(err.Error()),
			}
		case err.IsMissingParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.MissingParam(err.Error()),
			}
		case err.IsInvalidParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam(err.Error()),
			}
		default:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.Unknown(err.Error()),
			}
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func UploadCrossSigningDeviceSignatures(req *http.Request, keyserverAPI api.ClientKeyAPI, device *api.Device) util.JSONResponse {
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
				JSON: spec.InvalidSignature(err.Error()),
			}
		case err.IsMissingParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.MissingParam(err.Error()),
			}
		case err.IsInvalidParam:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam(err.Error()),
			}
		default:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.Unknown(err.Error()),
			}
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
