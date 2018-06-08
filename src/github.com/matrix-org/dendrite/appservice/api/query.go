// Copyright 2018 New Vector Ltd
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

// Package api contains methods used by dendrite components in multi-process
// mode to send requests to the appservice component, typically in order to ask
// an application service for some information.
package api

import (
	"context"
	"net/http"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	opentracing "github.com/opentracing/opentracing-go"
)

// RoomAliasExistsRequest is a request to an application service
// about whether a room alias exists
type RoomAliasExistsRequest struct {
	// Alias we want to lookup
	Alias string `json:"alias"`
}

// RoomAliasExistsResponse is a response from an application service
// about whether a room alias exists
type RoomAliasExistsResponse struct {
	AliasExists bool `json:"exists"`
}

// AppServiceQueryAPI is used to query user and room alias data from application
// services
type AppServiceQueryAPI interface {
	// Check whether a room alias exists within any application service namespaces
	RoomAliasExists(
		ctx context.Context,
		req *RoomAliasExistsRequest,
		response *RoomAliasExistsResponse,
	) error
	// TODO: QueryUserIDExists
}

// AppServiceRoomAliasExistsPath is the HTTP path for the RoomAliasExists API
const AppServiceRoomAliasExistsPath = "/api/appservice/RoomAliasExists"

// httpAppServiceQueryAPI contains the URL to an appservice query API and a
// reference to a httpClient used to reach it
type httpAppServiceQueryAPI struct {
	appserviceURL string
	httpClient    *http.Client
}

// NewAppServiceQueryAPIHTTP creates a AppServiceQueryAPI implemented by talking
// to a HTTP POST API.
// If httpClient is nil then it uses http.DefaultClient
func NewAppServiceQueryAPIHTTP(
	appserviceURL string,
	httpClient *http.Client,
) AppServiceQueryAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpAppServiceQueryAPI{appserviceURL, httpClient}
}

// RoomAliasExists implements AppServiceQueryAPI
func (h *httpAppServiceQueryAPI) RoomAliasExists(
	ctx context.Context,
	request *RoomAliasExistsRequest,
	response *RoomAliasExistsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "appserviceRoomAliasExists")
	defer span.Finish()

	apiURL := h.appserviceURL + AppServiceRoomAliasExistsPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
