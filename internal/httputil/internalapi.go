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
	"fmt"
	"net/http"
	"reflect"

	"github.com/matrix-org/util"
	opentracing "github.com/opentracing/opentracing-go"
)

type InternalAPIError struct {
	Type    string
	Message string
}

func (e InternalAPIError) Error() string {
	return fmt.Sprintf("internal API returned %q error: %s", e.Type, e.Message)
}

func MakeInternalRPCAPI[reqtype, restype any](metricsName string, f func(context.Context, *reqtype, *restype) error) http.Handler {
	return MakeInternalAPI(metricsName, func(req *http.Request) util.JSONResponse {
		var request reqtype
		var response restype
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			return util.MessageResponse(http.StatusBadRequest, err.Error())
		}
		if err := f(req.Context(), &request, &response); err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: &InternalAPIError{
					Type:    reflect.TypeOf(err).String(),
					Message: fmt.Sprintf("%s", err),
				},
			}
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: &response,
		}
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
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: err,
			}
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: response,
		}
	})
}

func CallInternalRPCAPI[reqtype, restype any](name, url string, client *http.Client, ctx context.Context, request *reqtype, response *restype) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, name)
	defer span.Finish()

	return PostJSON[reqtype, restype, InternalAPIError](ctx, span, client, url, request, response)
}

func CallInternalProxyAPI[reqtype, restype any, errtype error](name, url string, client *http.Client, ctx context.Context, request *reqtype) (restype, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, name)
	defer span.Finish()

	var response restype
	return response, PostJSON[reqtype, restype, errtype](ctx, span, client, url, request, &response)
}
