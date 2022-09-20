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
	"context"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/userapi/api"
)

const (
	PerformForgetSSOPath          = "/userapi/performForgetSSO"
	PerformSaveSSOAssociationPath = "/userapi/performSaveSSOAssociation"
	QueryLocalpartForSSOPath      = "/userapi/queryLocalpartForSSO"
)

func (h *httpUserInternalAPI) QueryLocalpartForSSO(ctx context.Context, request *api.QueryLocalpartForSSORequest, response *api.QueryLocalpartForSSOResponse) error {
	return httputil.CallInternalRPCAPI(
		"QuerytLocalpartForSSO", h.apiURL+QueryLocalpartForSSOPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformForgetSSO(ctx context.Context, request *api.PerformForgetSSORequest, response *struct{}) error {
	return httputil.CallInternalRPCAPI(
		"PerformForgetSSO", h.apiURL+PerformForgetSSOPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformSaveSSOAssociation(ctx context.Context, request *api.PerformSaveSSOAssociationRequest, response *struct{}) error {
	return httputil.CallInternalRPCAPI(
		"PerformSaveSSOAssociation", h.apiURL+PerformSaveSSOAssociationPath,
		h.httpClient, ctx, request, response,
	)
}
