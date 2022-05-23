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
	"github.com/opentracing/opentracing-go"
)

const (
	PerformForgetSSOPath          = "/userapi/performForgetSSO"
	PerformSaveSSOAssociationPath = "/userapi/performSaveSSOAssociation"
	QueryLocalpartForSSOPath      = "/userapi/queryLocalpartForSSO"
)

func (h *httpUserInternalAPI) QueryLocalpartForSSO(ctx context.Context, req *api.QueryLocalpartForSSORequest, res *api.QueryLocalpartForSSOResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, QueryLocalpartForSSOPath)
	defer span.Finish()

	apiURL := h.apiURL + QueryLocalpartForSSOPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) PerformForgetSSO(ctx context.Context, req *api.PerformForgetSSORequest, res *struct{}) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, PerformForgetSSOPath)
	defer span.Finish()

	apiURL := h.apiURL + PerformForgetSSOPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpUserInternalAPI) PerformSaveSSOAssociation(ctx context.Context, req *api.PerformSaveSSOAssociationRequest, res *struct{}) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, PerformSaveSSOAssociationPath)
	defer span.Finish()

	apiURL := h.apiURL + PerformSaveSSOAssociationPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}
