// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	PerformLoginTokenCreationPath = "/userapi/performLoginTokenCreation"
	PerformLoginTokenDeletionPath = "/userapi/performLoginTokenDeletion"
	QueryLoginTokenPath           = "/userapi/queryLoginToken"
)

func (h *httpUserInternalAPI) PerformLoginTokenCreation(
	ctx context.Context,
	request *api.PerformLoginTokenCreationRequest,
	response *api.PerformLoginTokenCreationResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformLoginTokenCreation", h.apiURL+PerformLoginTokenCreationPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) PerformLoginTokenDeletion(
	ctx context.Context,
	request *api.PerformLoginTokenDeletionRequest,
	response *api.PerformLoginTokenDeletionResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformLoginTokenDeletion", h.apiURL+PerformLoginTokenDeletionPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpUserInternalAPI) QueryLoginToken(
	ctx context.Context,
	request *api.QueryLoginTokenRequest,
	response *api.QueryLoginTokenResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryLoginToken", h.apiURL+QueryLoginTokenPath,
		h.httpClient, ctx, request, response,
	)
}
