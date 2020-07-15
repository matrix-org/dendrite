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
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/opentracing/opentracing-go"
)

// HTTP paths for the internal HTTP APIs
const (
	PerformUploadKeysPath = "/keyserver/performUploadKeys"
	PerformClaimKeysPath  = "/keyserver/performClaimKeys"
	QueryKeysPath         = "/keyserver/queryKeys"
)

// NewKeyServerClient creates a KeyInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewKeyServerClient(
	apiURL string,
	httpClient *http.Client,
) (api.KeyInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewKeyServerClient: httpClient is <nil>")
	}
	return &httpKeyInternalAPI{
		apiURL:     apiURL,
		httpClient: httpClient,
	}, nil
}

type httpKeyInternalAPI struct {
	apiURL     string
	httpClient *http.Client
}

func (h *httpKeyInternalAPI) PerformClaimKeys(
	ctx context.Context,
	request *api.PerformClaimKeysRequest,
	response *api.PerformClaimKeysResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformClaimKeys")
	defer span.Finish()

	apiURL := h.apiURL + PerformClaimKeysPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.Error = &api.KeyError{
			Err: err.Error(),
		}
	}
}

func (h *httpKeyInternalAPI) PerformUploadKeys(
	ctx context.Context,
	request *api.PerformUploadKeysRequest,
	response *api.PerformUploadKeysResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformUploadKeys")
	defer span.Finish()

	apiURL := h.apiURL + PerformUploadKeysPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.Error = &api.KeyError{
			Err: err.Error(),
		}
	}
}

func (h *httpKeyInternalAPI) QueryKeys(
	ctx context.Context,
	request *api.QueryKeysRequest,
	response *api.QueryKeysResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryKeys")
	defer span.Finish()

	apiURL := h.apiURL + QueryKeysPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.Error = &api.KeyError{
			Err: err.Error(),
		}
	}
}
