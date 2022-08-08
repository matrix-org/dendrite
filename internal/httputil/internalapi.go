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

	"github.com/matrix-org/util"
	opentracing "github.com/opentracing/opentracing-go"
)

func MakeInternalRPCAPI[reqtype, restype any](metricsName string, f func(context.Context, *reqtype, *restype) error) http.Handler {
	return MakeInternalAPI(metricsName, func(req *http.Request) util.JSONResponse {
		var request reqtype
		var response restype
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			return util.MessageResponse(http.StatusBadRequest, err.Error())
		}
		if err := f(req.Context(), &request, &response); err != nil {
			return util.ErrorResponse(err)
		}
		return util.JSONResponse{Code: http.StatusOK, JSON: &response}
	})
}

func MakeInternalProxyAPI[reqtype, restype any](metricsName string, f func(context.Context, *reqtype) (*restype, error)) http.Handler {
	return MakeInternalAPI(metricsName, func(req *http.Request) util.JSONResponse {
		var request reqtype
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			return util.MessageResponse(http.StatusBadRequest, err.Error())
		}
		response, err := f(req.Context(), &request)
		if err != nil {
			return util.ErrorResponse(err)
		}
		return util.JSONResponse{Code: http.StatusOK, JSON: response}
	})
}

func CallInternalRPCAPI[req, res any](name, url string, client *http.Client, ctx context.Context, request *req, response *res) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, name)
	defer span.Finish()

	return PostJSON[req, res, error](ctx, span, client, url, request, response)
}

func CallInternalProxyAPI[req, res any, errtype error](name, url string, client *http.Client, ctx context.Context, request *req) (res, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, name)
	defer span.Finish()

	var response res
	if err := PostJSON[req, res, errtype](ctx, span, client, url, request, &response); err != nil {
		return response, err
	}
	return response, nil
}
