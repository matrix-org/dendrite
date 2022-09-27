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
	Count     int64           `json:"count"`
	ETag      string          `json:"etag"`
	Version   string          `json:"version"`
}

type keyBackupSessionRequest struct {
	Rooms map[string]struct {
		Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
	} `json:"rooms"`
}

type keyBackupSessionResponse struct {
	Count int64  `json:"count"`
	ETag  string `json:"etag"`
}

// Create a new key backup. Request must contain a `keyBackupVersion`. Returns a `keyBackupVersionCreateResponse`.
// Implements  POST /_matrix/client/r0/room_keys/version
func CreateKeyBackupVersion(req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device) util.JSONResponse {
	var kb keyBackupVersion
	resErr := httputil.UnmarshalJSONRequest(req, &kb)
	if resErr != nil {
		return *resErr
	}
	var performKeyBackupResp userapi.PerformKeyBackupResponse
	if err := userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:    device.UserID,
		Version:   "",
		AuthData:  kb.AuthData,
		Algorithm: kb.Algorithm,
	}, &performKeyBackupResp); err != nil {
		return jsonerror.InternalServerError()
	}
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
func KeyBackupVersion(req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device, version string) util.JSONResponse {
	var queryResp userapi.QueryKeyBackupResponse
	if err := userAPI.QueryKeyBackup(req.Context(), &userapi.QueryKeyBackupRequest{
		UserID:  device.UserID,
		Version: version,
	}, &queryResp); err != nil {
		return jsonerror.InternalAPIError(req.Context(), err)
	}
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
func ModifyKeyBackupVersionAuthData(req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device, version string) util.JSONResponse {
	var kb keyBackupVersion
	resErr := httputil.UnmarshalJSONRequest(req, &kb)
	if resErr != nil {
		return *resErr
	}
	var performKeyBackupResp userapi.PerformKeyBackupResponse
	if err := userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:    device.UserID,
		Version:   version,
		AuthData:  kb.AuthData,
		Algorithm: kb.Algorithm,
	}, &performKeyBackupResp); err != nil {
		return jsonerror.InternalServerError()
	}
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
func DeleteKeyBackupVersion(req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device, version string) util.JSONResponse {
	var performKeyBackupResp userapi.PerformKeyBackupResponse
	if err := userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:       device.UserID,
		Version:      version,
		DeleteBackup: true,
	}, &performKeyBackupResp); err != nil {
		return jsonerror.InternalServerError()
	}
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

// Upload a bunch of session keys for a given `version`.
func UploadBackupKeys(
	req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device, version string, keys *keyBackupSessionRequest,
) util.JSONResponse {
	var performKeyBackupResp userapi.PerformKeyBackupResponse
	if err := userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:  device.UserID,
		Version: version,
		Keys:    *keys,
	}, &performKeyBackupResp); err != nil && performKeyBackupResp.Error == "" {
		return jsonerror.InternalServerError()
	}
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
	return util.JSONResponse{
		Code: 200,
		JSON: keyBackupSessionResponse{
			Count: performKeyBackupResp.KeyCount,
			ETag:  performKeyBackupResp.KeyETag,
		},
	}
}

// Get keys from a given backup version. Response returned varies depending on if roomID and sessionID are set.
func GetBackupKeys(
	req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device, version, roomID, sessionID string,
) util.JSONResponse {
	var queryResp userapi.QueryKeyBackupResponse
	if err := userAPI.QueryKeyBackup(req.Context(), &userapi.QueryKeyBackupRequest{
		UserID:           device.UserID,
		Version:          version,
		ReturnKeys:       true,
		KeysForRoomID:    roomID,
		KeysForSessionID: sessionID,
	}, &queryResp); err != nil {
		return jsonerror.InternalAPIError(req.Context(), err)
	}
	if queryResp.Error != "" {
		return util.ErrorResponse(fmt.Errorf("QueryKeyBackup: %s", queryResp.Error))
	}
	if !queryResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("version not found"),
		}
	}
	if sessionID != "" {
		// return the key itself if it was found
		roomData, ok := queryResp.Keys[roomID]
		if ok {
			key, ok := roomData[sessionID]
			if ok {
				return util.JSONResponse{
					Code: 200,
					JSON: key,
				}
			}
		}
	} else if roomID != "" {
		roomData, ok := queryResp.Keys[roomID]
		if ok {
			// wrap response in "sessions"
			return util.JSONResponse{
				Code: 200,
				JSON: struct {
					Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
				}{
					Sessions: roomData,
				},
			}
		}
	} else {
		// response is the same as the upload request
		var resp keyBackupSessionRequest
		resp.Rooms = make(map[string]struct {
			Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
		})
		for roomID, roomData := range queryResp.Keys {
			resp.Rooms[roomID] = struct {
				Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
			}{
				Sessions: roomData,
			}
		}
		return util.JSONResponse{
			Code: 200,
			JSON: resp,
		}
	}
	return util.JSONResponse{
		Code: 404,
		JSON: jsonerror.NotFound("keys not found"),
	}
}
