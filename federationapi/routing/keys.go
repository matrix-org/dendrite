// Copyright 2017 Vector Creations Ltd
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
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
)

type queryKeysRequest struct {
	DeviceKeys map[string][]string `json:"device_keys"`
}

// QueryDeviceKeys returns device keys for users on this server.
// https://matrix.org/docs/spec/server_server/latest#post-matrix-federation-v1-user-keys-query
func QueryDeviceKeys(
	httpReq *http.Request, request *gomatrixserverlib.FederationRequest, keyAPI api.FederationKeyAPI, thisServer gomatrixserverlib.ServerName,
) util.JSONResponse {
	var qkr queryKeysRequest
	err := json.Unmarshal(request.Content(), &qkr)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}
	// make sure we only query users on our domain
	for userID := range qkr.DeviceKeys {
		_, serverName, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil {
			delete(qkr.DeviceKeys, userID)
			continue // ignore invalid users
		}
		if serverName != thisServer {
			delete(qkr.DeviceKeys, userID)
			continue
		}
	}

	var queryRes api.QueryKeysResponse
	keyAPI.QueryKeys(httpReq.Context(), &api.QueryKeysRequest{
		UserToDevices: qkr.DeviceKeys,
	}, &queryRes)
	if queryRes.Error != nil {
		util.GetLogger(httpReq.Context()).WithError(queryRes.Error).Error("Failed to QueryKeys")
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct {
			DeviceKeys      interface{} `json:"device_keys"`
			MasterKeys      interface{} `json:"master_keys"`
			SelfSigningKeys interface{} `json:"self_signing_keys"`
		}{
			queryRes.DeviceKeys,
			queryRes.MasterKeys,
			queryRes.SelfSigningKeys,
		},
	}
}

type claimOTKsRequest struct {
	OneTimeKeys map[string]map[string]string `json:"one_time_keys"`
}

// ClaimOneTimeKeys claims OTKs for users on this server.
// https://matrix.org/docs/spec/server_server/latest#post-matrix-federation-v1-user-keys-claim
func ClaimOneTimeKeys(
	httpReq *http.Request, request *gomatrixserverlib.FederationRequest, keyAPI api.FederationKeyAPI, thisServer gomatrixserverlib.ServerName,
) util.JSONResponse {
	var cor claimOTKsRequest
	err := json.Unmarshal(request.Content(), &cor)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}
	// make sure we only claim users on our domain
	for userID := range cor.OneTimeKeys {
		_, serverName, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil {
			delete(cor.OneTimeKeys, userID)
			continue // ignore invalid users
		}
		if serverName != thisServer {
			delete(cor.OneTimeKeys, userID)
			continue
		}
	}

	var claimRes api.PerformClaimKeysResponse
	keyAPI.PerformClaimKeys(httpReq.Context(), &api.PerformClaimKeysRequest{
		OneTimeKeys: cor.OneTimeKeys,
	}, &claimRes)
	if claimRes.Error != nil {
		util.GetLogger(httpReq.Context()).WithError(claimRes.Error).Error("Failed to PerformClaimKeys")
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct {
			OneTimeKeys interface{} `json:"one_time_keys"`
		}{claimRes.OneTimeKeys},
	}
}

// LocalKeys returns the local keys for the server.
// See https://matrix.org/docs/spec/server_server/unstable.html#publishing-keys
func LocalKeys(cfg *config.FederationAPI) util.JSONResponse {
	keys, err := localKeys(cfg, time.Now().Add(cfg.Matrix.KeyValidityPeriod))
	if err != nil {
		return util.ErrorResponse(err)
	}
	return util.JSONResponse{Code: http.StatusOK, JSON: keys}
}

func localKeys(cfg *config.FederationAPI, validUntil time.Time) (*gomatrixserverlib.ServerKeys, error) {
	var keys gomatrixserverlib.ServerKeys

	keys.ServerName = cfg.Matrix.ServerName
	keys.ValidUntilTS = gomatrixserverlib.AsTimestamp(validUntil)

	publicKey := cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey)

	keys.VerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
		cfg.Matrix.KeyID: {
			Key: gomatrixserverlib.Base64Bytes(publicKey),
		},
	}

	keys.OldVerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.OldVerifyKey{}
	for _, oldVerifyKey := range cfg.Matrix.OldVerifyKeys {
		keys.OldVerifyKeys[oldVerifyKey.KeyID] = gomatrixserverlib.OldVerifyKey{
			VerifyKey: gomatrixserverlib.VerifyKey{
				Key: gomatrixserverlib.Base64Bytes(oldVerifyKey.PrivateKey.Public().(ed25519.PublicKey)),
			},
			ExpiredTS: oldVerifyKey.ExpiredAt,
		}
	}

	toSign, err := json.Marshal(keys.ServerKeyFields)
	if err != nil {
		return nil, err
	}

	keys.Raw, err = gomatrixserverlib.SignJSON(
		string(cfg.Matrix.ServerName), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey, toSign,
	)
	if err != nil {
		return nil, err
	}

	return &keys, nil
}

func NotaryKeys(
	httpReq *http.Request, cfg *config.FederationAPI,
	fsAPI federationAPI.FederationInternalAPI,
	req *gomatrixserverlib.PublicKeyNotaryLookupRequest,
) util.JSONResponse {
	if req == nil {
		req = &gomatrixserverlib.PublicKeyNotaryLookupRequest{}
		if reqErr := httputil.UnmarshalJSONRequest(httpReq, &req); reqErr != nil {
			return *reqErr
		}
	}

	var response struct {
		ServerKeys []json.RawMessage `json:"server_keys"`
	}
	response.ServerKeys = []json.RawMessage{}

	for serverName, kidToCriteria := range req.ServerKeys {
		var keyList []gomatrixserverlib.ServerKeys
		if serverName == cfg.Matrix.ServerName {
			if k, err := localKeys(cfg, time.Now().Add(cfg.Matrix.KeyValidityPeriod)); err == nil {
				keyList = append(keyList, *k)
			} else {
				return util.ErrorResponse(err)
			}
		} else {
			var resp federationAPI.QueryServerKeysResponse
			err := fsAPI.QueryServerKeys(httpReq.Context(), &federationAPI.QueryServerKeysRequest{
				ServerName:      serverName,
				KeyIDToCriteria: kidToCriteria,
			}, &resp)
			if err != nil {
				return util.ErrorResponse(err)
			}
			keyList = append(keyList, resp.ServerKeys...)
		}
		if len(keyList) == 0 {
			continue
		}

		for _, keys := range keyList {
			j, err := json.Marshal(keys)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to marshal %q response", serverName)
				return jsonerror.InternalServerError()
			}

			js, err := gomatrixserverlib.SignJSON(
				string(cfg.Matrix.ServerName), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey, j,
			)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to sign %q response", serverName)
				return jsonerror.InternalServerError()
			}

			response.ServerKeys = append(response.ServerKeys, js)
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}
