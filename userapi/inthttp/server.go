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
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// nolint: gocyclo
func AddRoutes(internalAPIMux *mux.Router, s api.UserInternalAPI) {
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
	internalAPIMux.Handle(PerformAccountCreationPath,
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
	internalAPIMux.Handle(QueryDeviceInfosPath,
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
}
