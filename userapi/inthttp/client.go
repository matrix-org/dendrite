// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/userapi/api"
)

// HTTP paths for the internal HTTP APIs
const (
	InputAccountDataPath = "/userapi/inputAccountData"

	PerformDeviceCreationPath          = "/userapi/performDeviceCreation"
	PerformAccountCreationPath         = "/userapi/performAccountCreation"
	PerformPasswordUpdatePath          = "/userapi/performPasswordUpdate"
	PerformDeviceDeletionPath          = "/userapi/performDeviceDeletion"
	PerformLastSeenUpdatePath          = "/userapi/performLastSeenUpdate"
	PerformDeviceUpdatePath            = "/userapi/performDeviceUpdate"
	PerformAccountDeactivationPath     = "/userapi/performAccountDeactivation"
	PerformOpenIDTokenCreationPath     = "/userapi/performOpenIDTokenCreation"
	PerformKeyBackupPath               = "/userapi/performKeyBackup"
	PerformPusherSetPath               = "/pushserver/performPusherSet"
	PerformPusherDeletionPath          = "/pushserver/performPusherDeletion"
	PerformPushRulesPutPath            = "/pushserver/performPushRulesPut"
	PerformSetAvatarURLPath            = "/userapi/performSetAvatarURL"
	PerformSetDisplayNamePath          = "/userapi/performSetDisplayName"
	PerformForgetThreePIDPath          = "/userapi/performForgetThreePID"
	PerformSaveThreePIDAssociationPath = "/userapi/performSaveThreePIDAssociation"

	QueryKeyBackupPath             = "/userapi/queryKeyBackup"
	QueryProfilePath               = "/userapi/queryProfile"
	QueryAccessTokenPath           = "/userapi/queryAccessToken"
	QueryDevicesPath               = "/userapi/queryDevices"
	QueryAccountDataPath           = "/userapi/queryAccountData"
	QueryDeviceInfosPath           = "/userapi/queryDeviceInfos"
	QuerySearchProfilesPath        = "/userapi/querySearchProfiles"
	QueryOpenIDTokenPath           = "/userapi/queryOpenIDToken"
	QueryPushersPath               = "/pushserver/queryPushers"
	QueryPushRulesPath             = "/pushserver/queryPushRules"
	QueryNotificationsPath         = "/pushserver/queryNotifications"
	QueryNumericLocalpartPath      = "/userapi/queryNumericLocalpart"
	QueryAccountAvailabilityPath   = "/userapi/queryAccountAvailability"
	QueryAccountByPasswordPath     = "/userapi/queryAccountByPassword"
	QueryLocalpartForThreePIDPath  = "/userapi/queryLocalpartForThreePID"
	QueryThreePIDsForLocalpartPath = "/userapi/queryThreePIDsForLocalpart"
)

// NewUserAPIClient creates a UserInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewUserAPIClient(
	apiURL string,
	httpClient *http.Client,
) (api.UserInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewUserAPIClient: httpClient is <nil>")
	}
	return &httpUserInternalAPI{
		apiURL:     apiURL,
		httpClient: httpClient,
	}, nil
}

type httpUserInternalAPI struct {
	apiURL     string
	httpClient *http.Client
}

func (h *httpUserInternalAPI) InputAccountData(ctx context.Context, req *api.InputAccountDataRequest, res *api.InputAccountDataResponse) error {
	return httputil.CallInternalRPCAPI(
		"InputAccountData", h.apiURL+InputAccountDataPath,
		h.httpClient, ctx, req, res,
	)
}

func (h *httpUserInternalAPI) PerformAccountCreation(
	ctx context.Context,
	request *api.PerformAccountCreationRequest,
	response *api.PerformAccountCreationResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformAccountCreation", h.apiURL+PerformAccountCreationPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformPasswordUpdate(
	ctx context.Context,
	request *api.PerformPasswordUpdateRequest,
	response *api.PerformPasswordUpdateResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformPasswordUpdate", h.apiURL+PerformPasswordUpdatePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformDeviceCreation(
	ctx context.Context,
	request *api.PerformDeviceCreationRequest,
	response *api.PerformDeviceCreationResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformDeviceCreation", h.apiURL+PerformDeviceCreationPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformDeviceDeletion(
	ctx context.Context,
	request *api.PerformDeviceDeletionRequest,
	response *api.PerformDeviceDeletionResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformDeviceDeletion", h.apiURL+PerformDeviceDeletionPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformLastSeenUpdate(
	ctx context.Context,
	request *api.PerformLastSeenUpdateRequest,
	response *api.PerformLastSeenUpdateResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformLastSeen", h.apiURL+PerformLastSeenUpdatePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformDeviceUpdate(
	ctx context.Context,
	request *api.PerformDeviceUpdateRequest,
	response *api.PerformDeviceUpdateResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformDeviceUpdate", h.apiURL+PerformDeviceUpdatePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformAccountDeactivation(
	ctx context.Context,
	request *api.PerformAccountDeactivationRequest,
	response *api.PerformAccountDeactivationResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformAccountDeactivation", h.apiURL+PerformAccountDeactivationPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformOpenIDTokenCreation(
	ctx context.Context,
	request *api.PerformOpenIDTokenCreationRequest,
	response *api.PerformOpenIDTokenCreationResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformOpenIDTokenCreation", h.apiURL+PerformOpenIDTokenCreationPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryProfile(
	ctx context.Context,
	request *api.QueryProfileRequest,
	response *api.QueryProfileResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryProfile", h.apiURL+QueryProfilePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryDeviceInfos(
	ctx context.Context,
	request *api.QueryDeviceInfosRequest,
	response *api.QueryDeviceInfosResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryDeviceInfos", h.apiURL+QueryDeviceInfosPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryAccessToken(
	ctx context.Context,
	request *api.QueryAccessTokenRequest,
	response *api.QueryAccessTokenResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryAccessToken", h.apiURL+QueryAccessTokenPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryDevices(
	ctx context.Context,
	request *api.QueryDevicesRequest,
	response *api.QueryDevicesResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryDevices", h.apiURL+QueryDevicesPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryAccountData(
	ctx context.Context,
	request *api.QueryAccountDataRequest,
	response *api.QueryAccountDataResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryAccountData", h.apiURL+QueryAccountDataPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QuerySearchProfiles(
	ctx context.Context,
	request *api.QuerySearchProfilesRequest,
	response *api.QuerySearchProfilesResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QuerySearchProfiles", h.apiURL+QuerySearchProfilesPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryOpenIDToken(
	ctx context.Context,
	request *api.QueryOpenIDTokenRequest,
	response *api.QueryOpenIDTokenResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryOpenIDToken", h.apiURL+QueryOpenIDTokenPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformKeyBackup(
	ctx context.Context,
	request *api.PerformKeyBackupRequest,
	response *api.PerformKeyBackupResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformKeyBackup", h.apiURL+PerformKeyBackupPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryKeyBackup(
	ctx context.Context,
	request *api.QueryKeyBackupRequest,
	response *api.QueryKeyBackupResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryKeyBackup", h.apiURL+QueryKeyBackupPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryNotifications(
	ctx context.Context,
	request *api.QueryNotificationsRequest,
	response *api.QueryNotificationsResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryNotifications", h.apiURL+QueryNotificationsPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformPusherSet(
	ctx context.Context,
	request *api.PerformPusherSetRequest,
	response *struct{},
) error {
	return httputil.CallInternalRPCAPI(
		"PerformPusherSet", h.apiURL+PerformPusherSetPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformPusherDeletion(
	ctx context.Context,
	request *api.PerformPusherDeletionRequest,
	response *struct{},
) error {
	return httputil.CallInternalRPCAPI(
		"PerformPusherDeletion", h.apiURL+PerformPusherDeletionPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryPushers(
	ctx context.Context,
	request *api.QueryPushersRequest,
	response *api.QueryPushersResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryPushers", h.apiURL+QueryPushersPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformPushRulesPut(
	ctx context.Context,
	request *api.PerformPushRulesPutRequest,
	response *struct{},
) error {
	return httputil.CallInternalRPCAPI(
		"PerformPushRulesPut", h.apiURL+PerformPushRulesPutPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryPushRules(
	ctx context.Context,
	request *api.QueryPushRulesRequest,
	response *api.QueryPushRulesResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryPushRules", h.apiURL+QueryPushRulesPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) SetAvatarURL(
	ctx context.Context,
	request *api.PerformSetAvatarURLRequest,
	response *api.PerformSetAvatarURLResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"SetAvatarURL", h.apiURL+PerformSetAvatarURLPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryNumericLocalpart(
	ctx context.Context,
	request *api.QueryNumericLocalpartRequest,
	response *api.QueryNumericLocalpartResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryNumericLocalpart", h.apiURL+QueryNumericLocalpartPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryAccountAvailability(
	ctx context.Context,
	request *api.QueryAccountAvailabilityRequest,
	response *api.QueryAccountAvailabilityResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryAccountAvailability", h.apiURL+QueryAccountAvailabilityPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryAccountByPassword(
	ctx context.Context,
	request *api.QueryAccountByPasswordRequest,
	response *api.QueryAccountByPasswordResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryAccountByPassword", h.apiURL+QueryAccountByPasswordPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) SetDisplayName(
	ctx context.Context,
	request *api.PerformUpdateDisplayNameRequest,
	response *api.PerformUpdateDisplayNameResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"SetDisplayName", h.apiURL+PerformSetDisplayNamePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryLocalpartForThreePID(
	ctx context.Context,
	request *api.QueryLocalpartForThreePIDRequest,
	response *api.QueryLocalpartForThreePIDResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryLocalpartForThreePID", h.apiURL+QueryLocalpartForThreePIDPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryThreePIDsForLocalpart(
	ctx context.Context,
	request *api.QueryThreePIDsForLocalpartRequest,
	response *api.QueryThreePIDsForLocalpartResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryThreePIDsForLocalpart", h.apiURL+QueryThreePIDsForLocalpartPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformForgetThreePID(
	ctx context.Context,
	request *api.PerformForgetThreePIDRequest,
	response *struct{},
) error {
	return httputil.CallInternalRPCAPI(
		"PerformForgetThreePID", h.apiURL+PerformForgetThreePIDPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformSaveThreePIDAssociation(
	ctx context.Context,
	request *api.PerformSaveThreePIDAssociationRequest,
	response *struct{},
) error {
	return httputil.CallInternalRPCAPI(
		"PerformSaveThreePIDAssociation", h.apiURL+PerformSaveThreePIDAssociationPath,
		h.httpClient, ctx, request, response,
	)
}
