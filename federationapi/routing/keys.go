// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"
	"time"

	clienthttputil "github.com/element-hq/dendrite/clientapi/httputil"
	federationAPI "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
	httpReq *http.Request, request *fclient.FederationRequest, keyAPI api.FederationKeyAPI, thisServer spec.ServerName,
) util.JSONResponse {
	var qkr queryKeysRequest
	err := json.Unmarshal(request.Content(), &qkr)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
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
	httpReq *http.Request, request *fclient.FederationRequest, keyAPI api.FederationKeyAPI, thisServer spec.ServerName,
) util.JSONResponse {
	var cor claimOTKsRequest
	err := json.Unmarshal(request.Content(), &cor)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
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
func LocalKeys(cfg *config.FederationAPI, serverName spec.ServerName) util.JSONResponse {
	keys, err := localKeys(cfg, serverName)
	if err != nil {
		return util.MessageResponse(http.StatusNotFound, err.Error())
	}
	return util.JSONResponse{Code: http.StatusOK, JSON: keys}
}

func localKeys(cfg *config.FederationAPI, serverName spec.ServerName) (*gomatrixserverlib.ServerKeys, error) {
	var keys gomatrixserverlib.ServerKeys
	var identity *fclient.SigningIdentity
	var err error
	if virtualHost := cfg.Matrix.VirtualHostForHTTPHost(serverName); virtualHost == nil {
		if identity, err = cfg.Matrix.SigningIdentityFor(cfg.Matrix.ServerName); err != nil {
			return nil, err
		}
		publicKey := cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey)
		keys.ServerName = cfg.Matrix.ServerName
		keys.ValidUntilTS = spec.AsTimestamp(time.Now().Add(cfg.Matrix.KeyValidityPeriod))
		keys.VerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
			cfg.Matrix.KeyID: {
				Key: spec.Base64Bytes(publicKey),
			},
		}
		keys.OldVerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.OldVerifyKey{}
		for _, oldVerifyKey := range cfg.Matrix.OldVerifyKeys {
			keys.OldVerifyKeys[oldVerifyKey.KeyID] = gomatrixserverlib.OldVerifyKey{
				VerifyKey: gomatrixserverlib.VerifyKey{
					Key: oldVerifyKey.PublicKey,
				},
				ExpiredTS: oldVerifyKey.ExpiredAt,
			}
		}
	} else {
		if identity, err = cfg.Matrix.SigningIdentityFor(virtualHost.ServerName); err != nil {
			return nil, err
		}
		publicKey := virtualHost.PrivateKey.Public().(ed25519.PublicKey)
		keys.ServerName = virtualHost.ServerName
		keys.ValidUntilTS = spec.AsTimestamp(time.Now().Add(virtualHost.KeyValidityPeriod))
		keys.VerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
			virtualHost.KeyID: {
				Key: spec.Base64Bytes(publicKey),
			},
		}
		// TODO: Virtual hosts probably want to be able to specify old signing
		// keys too, just in case
	}

	toSign, err := json.Marshal(keys.ServerKeyFields)
	if err != nil {
		return nil, err
	}

	keys.Raw, err = gomatrixserverlib.SignJSON(
		string(identity.ServerName), identity.KeyID, identity.PrivateKey, toSign,
	)
	return &keys, err
}

type NotaryKeysResponse struct {
	ServerKeys []json.RawMessage `json:"server_keys"`
}

func NotaryKeys(
	httpReq *http.Request, cfg *config.FederationAPI,
	fsAPI federationAPI.FederationInternalAPI,
	req *gomatrixserverlib.PublicKeyNotaryLookupRequest,
) util.JSONResponse {
	serverName := spec.ServerName(httpReq.Host) // TODO: this is not ideal
	if !cfg.Matrix.IsLocalServerName(serverName) {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Server name not known"),
		}
	}

	if req == nil {
		req = &gomatrixserverlib.PublicKeyNotaryLookupRequest{}
		if reqErr := clienthttputil.UnmarshalJSONRequest(httpReq, &req); reqErr != nil {
			return *reqErr
		}
	}

	response := NotaryKeysResponse{
		ServerKeys: []json.RawMessage{},
	}

	for serverName, kidToCriteria := range req.ServerKeys {
		var keyList []gomatrixserverlib.ServerKeys
		if serverName == cfg.Matrix.ServerName {
			if k, err := localKeys(cfg, serverName); err == nil {
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
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}

			js, err := gomatrixserverlib.SignJSON(
				string(cfg.Matrix.ServerName), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey, j,
			)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to sign %q response", serverName)
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}

			response.ServerKeys = append(response.ServerKeys, js)
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}
