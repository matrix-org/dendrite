// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/matrix-org/util"

	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type uploadKeysRequest struct {
	DeviceKeys  json.RawMessage            `json:"device_keys"`
	OneTimeKeys map[string]json.RawMessage `json:"one_time_keys"`
}

func UploadKeys(req *http.Request, keyAPI api.ClientKeyAPI, device *api.Device) util.JSONResponse {
	var r uploadKeysRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	uploadReq := &api.PerformUploadKeysRequest{
		DeviceID: device.ID,
		UserID:   device.UserID,
	}
	if r.DeviceKeys != nil {
		uploadReq.DeviceKeys = []api.DeviceKeys{
			{
				DeviceID: device.ID,
				UserID:   device.UserID,
				KeyJSON:  r.DeviceKeys,
			},
		}
	}
	if r.OneTimeKeys != nil {
		uploadReq.OneTimeKeys = []api.OneTimeKeys{
			{
				DeviceID: device.ID,
				UserID:   device.UserID,
				KeyJSON:  r.OneTimeKeys,
			},
		}
	}

	var uploadRes api.PerformUploadKeysResponse
	if err := keyAPI.PerformUploadKeys(req.Context(), uploadReq, &uploadRes); err != nil {
		return util.ErrorResponse(err)
	}
	if uploadRes.Error != nil {
		util.GetLogger(req.Context()).WithError(uploadRes.Error).Error("Failed to PerformUploadKeys")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if len(uploadRes.KeyErrors) > 0 {
		util.GetLogger(req.Context()).WithField("key_errors", uploadRes.KeyErrors).Error("Failed to upload one or more keys")
		return util.JSONResponse{
			Code: 400,
			JSON: uploadRes.KeyErrors,
		}
	}
	keyCount := make(map[string]int)
	if len(uploadRes.OneTimeKeyCounts) > 0 {
		keyCount = uploadRes.OneTimeKeyCounts[0].KeyCount
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct {
			OTKCounts interface{} `json:"one_time_key_counts"`
		}{keyCount},
	}
}

type queryKeysRequest struct {
	Timeout    int                 `json:"timeout"`
	DeviceKeys map[string][]string `json:"device_keys"`
}

func (r *queryKeysRequest) GetTimeout() time.Duration {
	if r.Timeout == 0 {
		return 10 * time.Second
	}
	timeout := time.Duration(r.Timeout) * time.Millisecond
	if timeout > time.Second*20 {
		timeout = time.Second * 20
	}
	return timeout
}

func QueryKeys(req *http.Request, keyAPI api.ClientKeyAPI, device *api.Device) util.JSONResponse {
	var r queryKeysRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}
	queryRes := api.QueryKeysResponse{}
	keyAPI.QueryKeys(req.Context(), &api.QueryKeysRequest{
		UserID:        device.UserID,
		UserToDevices: r.DeviceKeys,
		Timeout:       r.GetTimeout(),
	}, &queryRes)
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"device_keys":       queryRes.DeviceKeys,
			"master_keys":       queryRes.MasterKeys,
			"self_signing_keys": queryRes.SelfSigningKeys,
			"user_signing_keys": queryRes.UserSigningKeys,
			"failures":          queryRes.Failures,
		},
	}
}

type claimKeysRequest struct {
	TimeoutMS int `json:"timeout"`
	//  The keys to be claimed. A map from user ID, to a map from device ID to algorithm name.
	OneTimeKeys map[string]map[string]string `json:"one_time_keys"`
}

func (r *claimKeysRequest) GetTimeout() time.Duration {
	if r.TimeoutMS == 0 {
		return 10 * time.Second
	}
	return time.Duration(r.TimeoutMS) * time.Millisecond
}

func ClaimKeys(req *http.Request, keyAPI api.ClientKeyAPI) util.JSONResponse {
	var r claimKeysRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}
	claimRes := api.PerformClaimKeysResponse{}
	keyAPI.PerformClaimKeys(req.Context(), &api.PerformClaimKeysRequest{
		OneTimeKeys: r.OneTimeKeys,
		Timeout:     r.GetTimeout(),
	}, &claimRes)
	if claimRes.Error != nil {
		util.GetLogger(req.Context()).WithError(claimRes.Error).Error("failed to PerformClaimKeys")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"one_time_keys": claimRes.OneTimeKeys,
			"failures":      claimRes.Failures,
		},
	}
}
