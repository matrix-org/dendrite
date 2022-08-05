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

package httputil

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/util"
	opentracing "github.com/opentracing/opentracing-go"
)

type InternalAPIClient[req, res any] struct {
	name   string
	url    string
	client *http.Client
}

func NewInternalAPIClient[req, res any](name, url string, httpClient *http.Client) *InternalAPIClient[req, res] {
	return &InternalAPIClient[req, res]{
		name:   name,
		url:    url,
		client: httpClient,
	}
}

func (h *InternalAPIClient[req, res]) Call(ctx context.Context, request *req, response *res) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, h.name)
	defer span.Finish()

	return PostJSON(ctx, span, h.client, h.url, request, response)
}

type InternalAPIServer[req, res any] struct {
	name string
	url  string
	f    func(context.Context, *req, *res) error
}

func NewInternalAPIServer[req, res any](name, url string, f func(context.Context, *req, *res) error) *InternalAPIServer[req, res] {
	return &InternalAPIServer[req, res]{
		name: name,
		url:  url,
		f:    f,
	}
}

func (h *InternalAPIServer[req, res]) Serve(mux *mux.Router) {
	mux.Handle(
		h.url,
		MakeInternalAPI(h.name, func(httpReq *http.Request) util.JSONResponse {
			var request req
			var response res
			if err := json.NewDecoder(httpReq.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := h.f(httpReq.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
