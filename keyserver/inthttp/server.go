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
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/util"
)

func AddRoutes(internalAPIMux *mux.Router, s api.KeyInternalAPI) {
	internalAPIMux.Handle(PerformClaimKeysPath,
		httputil.MakeInternalAPI("performClaimKeys", func(req *http.Request) util.JSONResponse {
			request := api.PerformClaimKeysRequest{}
			response := api.PerformClaimKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			s.PerformClaimKeys(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformUploadKeysPath,
		httputil.MakeInternalAPI("performUploadKeys", func(req *http.Request) util.JSONResponse {
			request := api.PerformUploadKeysRequest{}
			response := api.PerformUploadKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			s.PerformUploadKeys(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryKeysPath,
		httputil.MakeInternalAPI("queryKeys", func(req *http.Request) util.JSONResponse {
			request := api.QueryKeysRequest{}
			response := api.QueryKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			s.QueryKeys(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryKeyChangesPath,
		httputil.MakeInternalAPI("queryKeyChanges", func(req *http.Request) util.JSONResponse {
			request := api.QueryKeyChangesRequest{}
			response := api.QueryKeyChangesResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			s.QueryKeyChanges(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
