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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type keyBackupVersion struct {
	Algorithm string          `json:"algorithm"`
	AuthData  json.RawMessage `json:"auth_data"`
}

type keyBackupVersionCreateResponse struct {
	Version string `json:"version"`
}

type keyBackupVersionResponse struct {
	Algorithm string          `json:"algorithm"`
	AuthData  json.RawMessage `json:"auth_data"`
	Count     int             `json:"count"`
	ETag      string          `json:"etag"`
	Version   string          `json:"version"`
}

// Create a new key backup. Request must contain a `keyBackupVersion`. Returns a `keyBackupVersionCreateResponse`.
// Implements  POST /_matrix/client/r0/room_keys/version
func CreateKeyBackupVersion(req *http.Request, userAPI userapi.UserInternalAPI, device *userapi.Device) util.JSONResponse {
	var kb keyBackupVersion
	resErr := httputil.UnmarshalJSONRequest(req, &kb)
	if resErr != nil {
		return *resErr
	}
	var performKeyBackupResp userapi.PerformKeyBackupResponse
	userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:    device.UserID,
		Version:   "",
		AuthData:  kb.AuthData,
		Algorithm: kb.Algorithm,
	}, &performKeyBackupResp)
	if performKeyBackupResp.Error != "" {
		if performKeyBackupResp.BadInput {
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.InvalidArgumentValue(performKeyBackupResp.Error),
			}
		}
		return util.ErrorResponse(fmt.Errorf("PerformKeyBackup: %s", performKeyBackupResp.Error))
	}
	return util.JSONResponse{
		Code: 200,
		JSON: keyBackupVersionCreateResponse{
			Version: performKeyBackupResp.Version,
		},
	}
}

// KeyBackupVersion returns the key backup version specified. If `version` is empty, the latest `keyBackupVersionResponse` is returned.
// Implements GET  /_matrix/client/r0/room_keys/version and GET /_matrix/client/r0/room_keys/version/{version}
func KeyBackupVersion(req *http.Request, userAPI userapi.UserInternalAPI, device *userapi.Device, version string) util.JSONResponse {
	var queryResp userapi.QueryKeyBackupResponse
	userAPI.QueryKeyBackup(req.Context(), &userapi.QueryKeyBackupRequest{}, &queryResp)
	if queryResp.Error != "" {
		return util.ErrorResponse(fmt.Errorf("QueryKeyBackup: %s", queryResp.Error))
	}
	if !queryResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("version not found"),
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: keyBackupVersionResponse{
			Algorithm: queryResp.Algorithm,
			AuthData:  queryResp.AuthData,
			Count:     queryResp.Count,
			ETag:      queryResp.ETag,
			Version:   queryResp.Version,
		},
	}
}

// Modify the auth data of a key backup. Version must not be empty. Request must contain a `keyBackupVersion`
// Implements PUT  /_matrix/client/r0/room_keys/version/{version}
func ModifyKeyBackupVersionAuthData(req *http.Request, userAPI userapi.UserInternalAPI, device *userapi.Device, version string) util.JSONResponse {
	var kb keyBackupVersion
	resErr := httputil.UnmarshalJSONRequest(req, &kb)
	if resErr != nil {
		return *resErr
	}
	var performKeyBackupResp userapi.PerformKeyBackupResponse
	userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:    device.UserID,
		Version:   version,
		AuthData:  kb.AuthData,
		Algorithm: kb.Algorithm,
	}, &performKeyBackupResp)
	if performKeyBackupResp.Error != "" {
		if performKeyBackupResp.BadInput {
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.InvalidArgumentValue(performKeyBackupResp.Error),
			}
		}
		return util.ErrorResponse(fmt.Errorf("PerformKeyBackup: %s", performKeyBackupResp.Error))
	}
	if !performKeyBackupResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("backup version not found"),
		}
	}
	// Unclear what the 200 body should be
	return util.JSONResponse{
		Code: 200,
		JSON: keyBackupVersionCreateResponse{
			Version: performKeyBackupResp.Version,
		},
	}
}

// Delete a version of key backup. Version must not be empty. If the key backup was previously deleted, will return 200 OK.
// Implements DELETE  /_matrix/client/r0/room_keys/version/{version}
func DeleteKeyBackupVersion(req *http.Request, userAPI userapi.UserInternalAPI, device *userapi.Device, version string) util.JSONResponse {
	var performKeyBackupResp userapi.PerformKeyBackupResponse
	userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:       device.UserID,
		Version:      version,
		DeleteBackup: true,
	}, &performKeyBackupResp)
	if performKeyBackupResp.Error != "" {
		if performKeyBackupResp.BadInput {
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.InvalidArgumentValue(performKeyBackupResp.Error),
			}
		}
		return util.ErrorResponse(fmt.Errorf("PerformKeyBackup: %s", performKeyBackupResp.Error))
	}
	if !performKeyBackupResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("backup version not found"),
		}
	}
	// Unclear what the 200 body should be
	return util.JSONResponse{
		Code: 200,
		JSON: keyBackupVersionCreateResponse{
			Version: performKeyBackupResp.Version,
		},
	}
}
