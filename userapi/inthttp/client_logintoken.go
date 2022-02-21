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
	"github.com/opentracing/opentracing-go"
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLoginTokenCreation")
	defer span.Finish()

	apiURL := h.apiURL + PerformLoginTokenCreationPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) PerformLoginTokenDeletion(
	ctx context.Context,
	request *api.PerformLoginTokenDeletionRequest,
	response *api.PerformLoginTokenDeletionResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLoginTokenDeletion")
	defer span.Finish()

	apiURL := h.apiURL + PerformLoginTokenDeletionPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpUserInternalAPI) QueryLoginToken(
	ctx context.Context,
	request *api.QueryLoginTokenRequest,
	response *api.QueryLoginTokenResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryLoginToken")
	defer span.Finish()

	apiURL := h.apiURL + QueryLoginTokenPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
