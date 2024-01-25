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
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
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

	//! GlobeKeeper Customization: If user was registered with appservice (like BridgeAS), then we allow it to upload keys without a password
	if device.AccountType != userapi.AccountTypeAppService {
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
			UserApi: accountAPI,
			Config:  cfg,
		}
		if _, authErr := typePassword.Login(req.Context(), &uploadReq.Auth.PasswordRequest); authErr != nil {
			return *authErr
		}
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

	// Following additional logic is implemented to follow the [Notion PRD](https://globekeeper.notion.site/Account-Data-State-Event-c64c8df8025a494d86d3137d4e080ece)
	if device.UserID != "" {
		prevAccountDataReq := api.QueryAccountDataRequest{
			UserID:   device.UserID,
			DataType: "account_data",
			RoomID:   "",
		}
		accountDataRes := api.QueryAccountDataResponse{}
		if err := accountAPI.QueryAccountData(req.Context(), &prevAccountDataReq, &accountDataRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("userAPI.QueryAccountData failed")
			return util.ErrorResponse(fmt.Errorf("userAPI.QueryAccountData: %w", err))
		}
		var accoundData api.AccountData
		if len(accountDataRes.GlobalAccountData) != 0 {
			err := json.Unmarshal(accountDataRes.GlobalAccountData["account_data"], &accoundData)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{Err: err.Error()},
				}
			}
		}
		accoundData.LatestKeysUploadTs = time.Now().UnixMilli()
		newAccountData, err := json.Marshal(accoundData)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{Err: err.Error()},
			}
		}

		dataReq := api.InputAccountDataRequest{
			UserID:      device.UserID,
			DataType:    "account_data",
			RoomID:      "",
			AccountData: json.RawMessage(newAccountData),
		}
		dataRes := api.InputAccountDataResponse{}
		if err := accountAPI.InputAccountData(req.Context(), &dataReq, &dataRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("userAPI.InputAccountData on LatestKeysUploadTs update failed")
			return util.ErrorResponse(err)
		}
		logger := util.GetLogger(req.Context()).WithField("user_id", device.UserID)
		logger.Info("updated latestKeysUploadTs field in account data")
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
