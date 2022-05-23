// Copyright 2022 The Matrix.org Foundation C.I.C.
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

// addRoutesSSO adds routes for all SSO API calls.
func addRoutesSSO(internalAPIMux *mux.Router, s api.UserInternalAPI) {
	internalAPIMux.Handle(QueryLocalpartForSSOPath,
		httputil.MakeInternalAPI("queryLocalpartForSSO", func(req *http.Request) util.JSONResponse {
			request := api.QueryLocalpartForSSORequest{}
			response := api.QueryLocalpartForSSOResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.QueryLocalpartForSSO(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformForgetSSOPath,
		httputil.MakeInternalAPI("performForgetSSO", func(req *http.Request) util.JSONResponse {
			request := api.PerformForgetSSORequest{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformForgetSSO(req.Context(), &request, &struct{}{}); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &struct{}{}}
		}),
	)
	internalAPIMux.Handle(PerformSaveSSOAssociationPath,
		httputil.MakeInternalAPI("performSaveSSOAssociation", func(req *http.Request) util.JSONResponse {
			request := api.PerformSaveSSOAssociationRequest{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := s.PerformSaveSSOAssociation(req.Context(), &request, &struct{}{}); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &struct{}{}}
		}),
	)
}
