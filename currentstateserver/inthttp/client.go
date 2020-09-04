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

	"github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/opentracing/opentracing-go"
)

// HTTP paths for the internal HTTP APIs
const (
	QueryRoomsForUserPath     = "/currentstateserver/queryRoomsForUser"
	QueryBulkStateContentPath = "/currentstateserver/queryBulkStateContent"
)

// NewCurrentStateAPIClient creates a CurrentStateInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewCurrentStateAPIClient(
	apiURL string,
	httpClient *http.Client,
) (api.CurrentStateInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewCurrentStateAPIClient: httpClient is <nil>")
	}
	return &httpCurrentStateInternalAPI{
		apiURL:     apiURL,
		httpClient: httpClient,
	}, nil
}

type httpCurrentStateInternalAPI struct {
	apiURL     string
	httpClient *http.Client
}

func (h *httpCurrentStateInternalAPI) QueryRoomsForUser(
	ctx context.Context,
	request *api.QueryRoomsForUserRequest,
	response *api.QueryRoomsForUserResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryRoomsForUser")
	defer span.Finish()

	apiURL := h.apiURL + QueryRoomsForUserPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpCurrentStateInternalAPI) QueryBulkStateContent(
	ctx context.Context,
	request *api.QueryBulkStateContentRequest,
	response *api.QueryBulkStateContentResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryBulkStateContent")
	defer span.Finish()

	apiURL := h.apiURL + QueryBulkStateContentPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
