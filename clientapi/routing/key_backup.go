// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/element-hq/dendrite/clientapi/httputil"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
	if len(kb.AuthData) == 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("missing auth_data"),
		}
	}
	version, err := userAPI.PerformKeyBackup(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:    device.UserID,
		Version:   "",
		AuthData:  kb.AuthData,
		Algorithm: kb.Algorithm,
	})
	if err != nil {
		return util.ErrorResponse(fmt.Errorf("PerformKeyBackup: %w", err))
	}

	return util.JSONResponse{
		Code: 200,
		JSON: keyBackupVersionCreateResponse{
			Version: version,
		},
	}
}

// KeyBackupVersion returns the key backup version specified. If `version` is empty, the latest `keyBackupVersionResponse` is returned.
// Implements GET  /_matrix/client/r0/room_keys/version and GET /_matrix/client/r0/room_keys/version/{version}
func KeyBackupVersion(req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device, version string) util.JSONResponse {
	queryResp, err := userAPI.QueryKeyBackup(req.Context(), &userapi.QueryKeyBackupRequest{
		UserID:  device.UserID,
		Version: version,
	})
	if err != nil {
		return util.ErrorResponse(fmt.Errorf("QueryKeyBackup: %s", err))
	}
	if !queryResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: spec.NotFound("version not found"),
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
	performKeyBackupResp, err := userAPI.UpdateBackupKeyAuthData(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:    device.UserID,
		Version:   version,
		AuthData:  kb.AuthData,
		Algorithm: kb.Algorithm,
	})
	switch e := err.(type) {
	case spec.ErrRoomKeysVersion:
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: e,
		}
	case nil:
	default:
		return util.ErrorResponse(fmt.Errorf("PerformKeyBackup: %w", e))
	}

	if !performKeyBackupResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: spec.NotFound("backup version not found"),
		}
	}
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
	exists, err := userAPI.DeleteKeyBackup(req.Context(), device.UserID, version)
	if err != nil {
		return util.ErrorResponse(fmt.Errorf("DeleteKeyBackup: %s", err))
	}
	if !exists {
		return util.JSONResponse{
			Code: 404,
			JSON: spec.NotFound("backup version not found"),
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

// Upload a bunch of session keys for a given `version`.
func UploadBackupKeys(
	req *http.Request, userAPI userapi.ClientUserAPI, device *userapi.Device, version string, keys *keyBackupSessionRequest,
) util.JSONResponse {
	performKeyBackupResp, err := userAPI.UpdateBackupKeyAuthData(req.Context(), &userapi.PerformKeyBackupRequest{
		UserID:  device.UserID,
		Version: version,
		Keys:    *keys,
	})

	switch e := err.(type) {
	case spec.ErrRoomKeysVersion:
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: e,
		}
	case nil:
	default:
		return util.ErrorResponse(fmt.Errorf("PerformKeyBackup: %w", e))
	}
	if !performKeyBackupResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: spec.NotFound("backup version not found"),
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
	queryResp, err := userAPI.QueryKeyBackup(req.Context(), &userapi.QueryKeyBackupRequest{
		UserID:           device.UserID,
		Version:          version,
		ReturnKeys:       true,
		KeysForRoomID:    roomID,
		KeysForSessionID: sessionID,
	})
	if err != nil {
		return util.ErrorResponse(fmt.Errorf("QueryKeyBackup: %w", err))
	}
	if !queryResp.Exists {
		return util.JSONResponse{
			Code: 404,
			JSON: spec.NotFound("version not found"),
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
		if !ok {
			// If no keys are found, then an object with an empty sessions property will be returned
			roomData = make(map[string]userapi.KeyBackupSession)
		}
		// wrap response in "sessions"
		return util.JSONResponse{
			Code: 200,
			JSON: struct {
				Sessions map[string]userapi.KeyBackupSession `json:"sessions"`
			}{
				Sessions: roomData,
			},
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
		JSON: spec.NotFound("keys not found"),
	}
}
