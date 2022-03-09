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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// nolint: gocyclo
func AddRoutes(internalAPIMux *mux.Router, s api.UserInternalAPI) {
	addRoutesLoginToken(internalAPIMux, s)

	internalAPIMux.Handle(PerformAccountCreationPath,
		httputil.MakeInternalAPI("performAccountCreation", func(req *http.Request) util.JSONResponse {
			request := api.PerformAccountCreationRequest{}
			response := api.PerformAccountCreationResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformAccountCreation(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformPasswordUpdatePath,
		httputil.MakeInternalAPI("performPasswordUpdate", func(req *http.Request) util.JSONResponse {
			request := api.PerformPasswordUpdateRequest{}
			response := api.PerformPasswordUpdateResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformPasswordUpdate(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformDeviceCreationPath,
		httputil.MakeInternalAPI("performDeviceCreation", func(req *http.Request) util.JSONResponse {
			request := api.PerformDeviceCreationRequest{}
			response := api.PerformDeviceCreationResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformDeviceCreation(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformLastSeenUpdatePath,
		httputil.MakeInternalAPI("performLastSeenUpdate", func(req *http.Request) util.JSONResponse {
			request := api.PerformLastSeenUpdateRequest{}
			response := api.PerformLastSeenUpdateResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformLastSeenUpdate(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformDeviceUpdatePath,
		httputil.MakeInternalAPI("performDeviceUpdate", func(req *http.Request) util.JSONResponse {
			request := api.PerformDeviceUpdateRequest{}
			response := api.PerformDeviceUpdateResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformDeviceUpdate(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformDeviceDeletionPath,
		httputil.MakeInternalAPI("performDeviceDeletion", func(req *http.Request) util.JSONResponse {
			request := api.PerformDeviceDeletionRequest{}
			response := api.PerformDeviceDeletionResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformDeviceDeletion(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformAccountDeactivationPath,
		httputil.MakeInternalAPI("performAccountDeactivation", func(req *http.Request) util.JSONResponse {
			request := api.PerformAccountDeactivationRequest{}
			response := api.PerformAccountDeactivationResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformAccountDeactivation(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformOpenIDTokenCreationPath,
		httputil.MakeInternalAPI("performOpenIDTokenCreation", func(req *http.Request) util.JSONResponse {
			request := api.PerformOpenIDTokenCreationRequest{}
			response := api.PerformOpenIDTokenCreationResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformOpenIDTokenCreation(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryProfilePath,
		httputil.MakeInternalAPI("queryProfile", func(req *http.Request) util.JSONResponse {
			request := api.QueryProfileRequest{}
			response := api.QueryProfileResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryProfile(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryAccessTokenPath,
		httputil.MakeInternalAPI("queryAccessToken", func(req *http.Request) util.JSONResponse {
			request := api.QueryAccessTokenRequest{}
			response := api.QueryAccessTokenResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryAccessToken(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryDevicesPath,
		httputil.MakeInternalAPI("queryDevices", func(req *http.Request) util.JSONResponse {
			request := api.QueryDevicesRequest{}
			response := api.QueryDevicesResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryDevices(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryAccountDataPath,
		httputil.MakeInternalAPI("queryAccountData", func(req *http.Request) util.JSONResponse {
			request := api.QueryAccountDataRequest{}
			response := api.QueryAccountDataResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryAccountData(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryDeviceInfosPath,
		httputil.MakeInternalAPI("queryDeviceInfos", func(req *http.Request) util.JSONResponse {
			request := api.QueryDeviceInfosRequest{}
			response := api.QueryDeviceInfosResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryDeviceInfos(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QuerySearchProfilesPath,
		httputil.MakeInternalAPI("querySearchProfiles", func(req *http.Request) util.JSONResponse {
			request := api.QuerySearchProfilesRequest{}
			response := api.QuerySearchProfilesResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QuerySearchProfiles(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryOpenIDTokenPath,
		httputil.MakeInternalAPI("queryOpenIDToken", func(req *http.Request) util.JSONResponse {
			request := api.QueryOpenIDTokenRequest{}
			response := api.QueryOpenIDTokenResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryOpenIDToken(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(InputAccountDataPath,
		httputil.MakeInternalAPI("inputAccountDataPath", func(req *http.Request) util.JSONResponse {
			request := api.InputAccountDataRequest{}
			response := api.InputAccountDataResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.InputAccountData(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryKeyBackupPath,
		httputil.MakeInternalAPI("queryKeyBackup", func(req *http.Request) util.JSONResponse {
			request := api.QueryKeyBackupRequest{}
			response := api.QueryKeyBackupResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			s.QueryKeyBackup(req.Context(), &request, &response)
			if response.Error != "" {
				return util.ErrorResponse(fmt.Errorf("QueryKeyBackup: %s", response.Error))
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformKeyBackupPath,
		httputil.MakeInternalAPI("performKeyBackup", func(req *http.Request) util.JSONResponse {
			request := api.PerformKeyBackupRequest{}
			response := api.PerformKeyBackupResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			err := s.PerformKeyBackup(req.Context(), &request, &response)
			if err != nil {
				return util.JSONResponse{Code: http.StatusBadRequest, JSON: &response}
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryNotificationsPath,
		httputil.MakeInternalAPI("queryNotifications", func(req *http.Request) util.JSONResponse {
			var request api.QueryNotificationsRequest
			var response api.QueryNotificationsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryNotifications(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)

	internalAPIMux.Handle(PerformPusherSetPath,
		httputil.MakeInternalAPI("performPusherSet", func(req *http.Request) util.JSONResponse {
			request := api.PerformPusherSetRequest{}
			response := struct{}{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformPusherSet(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformPusherDeletionPath,
		httputil.MakeInternalAPI("performPusherDeletion", func(req *http.Request) util.JSONResponse {
			request := api.PerformPusherDeletionRequest{}
			response := struct{}{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformPusherDeletion(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)

	internalAPIMux.Handle(QueryPushersPath,
		httputil.MakeInternalAPI("queryPushers", func(req *http.Request) util.JSONResponse {
			request := api.QueryPushersRequest{}
			response := api.QueryPushersResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryPushers(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)

	internalAPIMux.Handle(PerformPushRulesPutPath,
		httputil.MakeInternalAPI("performPushRulesPut", func(req *http.Request) util.JSONResponse {
			request := api.PerformPushRulesPutRequest{}
			response := struct{}{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformPushRulesPut(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)

	internalAPIMux.Handle(QueryPushRulesPath,
		httputil.MakeInternalAPI("queryPushRules", func(req *http.Request) util.JSONResponse {
			request := api.QueryPushRulesRequest{}
			response := api.QueryPushRulesResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryPushRules(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
