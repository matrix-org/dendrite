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
	"github.com/opentracing/opentracing-go"
)

// HTTP paths for the internal HTTP APIs
const (
	InputAccountDataPath = "/userapi/inputAccountData"

	PerformDeviceCreationPath      = "/userapi/performDeviceCreation"
	PerformAccountCreationPath     = "/userapi/performAccountCreation"
	PerformPasswordUpdatePath      = "/userapi/performPasswordUpdate"
	PerformDeviceDeletionPath      = "/userapi/performDeviceDeletion"
	PerformLastSeenUpdatePath      = "/userapi/performLastSeenUpdate"
	PerformDeviceUpdatePath        = "/userapi/performDeviceUpdate"
	PerformAccountDeactivationPath = "/userapi/performAccountDeactivation"
	PerformOpenIDTokenCreationPath = "/userapi/performOpenIDTokenCreation"
	PerformKeyBackupPath           = "/userapi/performKeyBackup"

	QueryKeyBackupPath      = "/userapi/queryKeyBackup"
	QueryProfilePath        = "/userapi/queryProfile"
	QueryAccessTokenPath    = "/userapi/queryAccessToken"
	QueryDevicesPath        = "/userapi/queryDevices"
	QueryAccountDataPath    = "/userapi/queryAccountData"
	QueryDeviceInfosPath    = "/userapi/queryDeviceInfos"
	QuerySearchProfilesPath = "/userapi/querySearchProfiles"
	QueryOpenIDTokenPath    = "/userapi/queryOpenIDToken"

	StatsAllUsers             = "/userapi/stats/allUsers"
	StatsNonBridgedUsers      = "/userapi/stats/nonBridgedUsers"
	StatsRegisteredUserByType = "/userapi/stats/registeredUserByType"
	StatsDailyUsers           = "/userapi/stats/dailyUsers"
	StatsMonthlyUsers         = "/userapi/stats/monthlyUsers"
	StatsR30Users             = "/userapi/stats/r30Users"
	StatsR30UsersV2           = "/userapi/stats/r30UsersV2"
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputAccountData")
	defer span.Finish()

	apiURL := h.apiURL + InputAccountDataPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) PerformAccountCreation(
	ctx context.Context,
	request *api.PerformAccountCreationRequest,
	response *api.PerformAccountCreationResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformAccountCreation")
	defer span.Finish()

	apiURL := h.apiURL + PerformAccountCreationPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) PerformPasswordUpdate(
	ctx context.Context,
	request *api.PerformPasswordUpdateRequest,
	response *api.PerformPasswordUpdateResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformPasswordUpdate")
	defer span.Finish()

	apiURL := h.apiURL + PerformPasswordUpdatePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) PerformDeviceCreation(
	ctx context.Context,
	request *api.PerformDeviceCreationRequest,
	response *api.PerformDeviceCreationResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformDeviceCreation")
	defer span.Finish()

	apiURL := h.apiURL + PerformDeviceCreationPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) PerformDeviceDeletion(
	ctx context.Context,
	request *api.PerformDeviceDeletionRequest,
	response *api.PerformDeviceDeletionResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformDeviceDeletion")
	defer span.Finish()

	apiURL := h.apiURL + PerformDeviceDeletionPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) PerformLastSeenUpdate(
	ctx context.Context,
	req *api.PerformLastSeenUpdateRequest,
	res *api.PerformLastSeenUpdateResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLastSeen")
	defer span.Finish()

	apiURL := h.apiURL + PerformLastSeenUpdatePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) PerformDeviceUpdate(ctx context.Context, req *api.PerformDeviceUpdateRequest, res *api.PerformDeviceUpdateResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformDeviceUpdate")
	defer span.Finish()

	apiURL := h.apiURL + PerformDeviceUpdatePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) PerformAccountDeactivation(ctx context.Context, req *api.PerformAccountDeactivationRequest, res *api.PerformAccountDeactivationResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformAccountDeactivation")
	defer span.Finish()

	apiURL := h.apiURL + PerformAccountDeactivationPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) PerformOpenIDTokenCreation(ctx context.Context, request *api.PerformOpenIDTokenCreationRequest, response *api.PerformOpenIDTokenCreationResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformOpenIDTokenCreation")
	defer span.Finish()

	apiURL := h.apiURL + PerformOpenIDTokenCreationPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) QueryProfile(
	ctx context.Context,
	request *api.QueryProfileRequest,
	response *api.QueryProfileResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryProfile")
	defer span.Finish()

	apiURL := h.apiURL + QueryProfilePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) QueryDeviceInfos(
	ctx context.Context,
	request *api.QueryDeviceInfosRequest,
	response *api.QueryDeviceInfosResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryDeviceInfos")
	defer span.Finish()

	apiURL := h.apiURL + QueryDeviceInfosPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) QueryAccessToken(
	ctx context.Context,
	request *api.QueryAccessTokenRequest,
	response *api.QueryAccessTokenResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryAccessToken")
	defer span.Finish()

	apiURL := h.apiURL + QueryAccessTokenPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) QueryDevices(ctx context.Context, req *api.QueryDevicesRequest, res *api.QueryDevicesResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryDevices")
	defer span.Finish()

	apiURL := h.apiURL + QueryDevicesPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) QueryAccountData(ctx context.Context, req *api.QueryAccountDataRequest, res *api.QueryAccountDataResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryAccountData")
	defer span.Finish()

	apiURL := h.apiURL + QueryAccountDataPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) QuerySearchProfiles(ctx context.Context, req *api.QuerySearchProfilesRequest, res *api.QuerySearchProfilesResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QuerySearchProfiles")
	defer span.Finish()

	apiURL := h.apiURL + QuerySearchProfilesPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) QueryOpenIDToken(ctx context.Context, req *api.QueryOpenIDTokenRequest, res *api.QueryOpenIDTokenResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryOpenIDToken")
	defer span.Finish()

	apiURL := h.apiURL + QueryOpenIDTokenPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) PerformKeyBackup(ctx context.Context, req *api.PerformKeyBackupRequest, res *api.PerformKeyBackupResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformKeyBackup")
	defer span.Finish()

	apiURL := h.apiURL + PerformKeyBackupPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
	if err != nil {
		res.Error = err.Error()
	}
	return nil
}
func (h *httpUserInternalAPI) QueryKeyBackup(ctx context.Context, req *api.QueryKeyBackupRequest, res *api.QueryKeyBackupResponse) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryKeyBackup")
	defer span.Finish()

	apiURL := h.apiURL + QueryKeyBackupPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
	if err != nil {
		res.Error = err.Error()
	}
}

func (h *httpUserInternalAPI) AllUsers(ctx context.Context, res *api.IntegerResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "AllUsers")
	defer span.Finish()

	apiURL := h.apiURL + StatsAllUsers
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, api.EmptyRequest{}, res)
}

func (h *httpUserInternalAPI) NonBridgedUsers(ctx context.Context, res *api.IntegerResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "NonBridgedUsers")
	defer span.Finish()

	apiURL := h.apiURL + StatsNonBridgedUsers
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, api.EmptyRequest{}, res)
}

func (h *httpUserInternalAPI) RegisteredUserByType(ctx context.Context, res *api.MapResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RegisteredUserByType")
	defer span.Finish()

	apiURL := h.apiURL + StatsRegisteredUserByType
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, api.EmptyRequest{}, res)
}

func (h *httpUserInternalAPI) DailyUsers(ctx context.Context, res *api.IntegerResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "DailyUsers")
	defer span.Finish()

	apiURL := h.apiURL + StatsDailyUsers
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, api.EmptyRequest{}, res)
}

func (h *httpUserInternalAPI) MonthlyUsers(ctx context.Context, res *api.IntegerResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "MonthlyUsers")
	defer span.Finish()

	apiURL := h.apiURL + StatsMonthlyUsers
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, api.EmptyRequest{}, res)
}

func (h *httpUserInternalAPI) R30Users(ctx context.Context, res *api.MapResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "R30Users")
	defer span.Finish()

	apiURL := h.apiURL + StatsR30Users
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, api.EmptyRequest{}, res)
}

func (h *httpUserInternalAPI) R30UsersV2(ctx context.Context, res *api.MapResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "R30UsersV2")
	defer span.Finish()

	apiURL := h.apiURL + StatsR30UsersV2
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, api.EmptyRequest{}, res)
}