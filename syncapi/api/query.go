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

package api

import (
	"context"
	"net/http"

	internalHTTP "github.com/matrix-org/dendrite/internal/http"
	"github.com/matrix-org/util"
	opentracing "github.com/opentracing/opentracing-go"
)

const (
	SyncAPIQuerySyncPath      = "/syncapi/querySync"
	SyncAPIQueryStatePath     = "/syncapi/queryState"
	SyncAPIQueryStateTypePath = "/syncapi/queryStateType"
	SyncAPIQueryMessagesPath  = "/syncapi/queryMessages"
)

func NewSyncQueryAPIHTTP(syncapiURL string, httpClient *http.Client) SyncQueryAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpSyncQueryAPI{syncapiURL, httpClient}
}

type httpSyncQueryAPI struct {
	syncapiURL string
	httpClient *http.Client
}

type SyncQueryAPI interface {
	QuerySync(ctx context.Context, request *QuerySyncRequest, response *QuerySyncResponse) error
	QueryState(ctx context.Context, request *QueryStateRequest, response *QueryStateResponse) error
	QueryStateType(ctx context.Context, request *QueryStateTypeRequest, response *QueryStateTypeResponse) error
	QueryMessages(ctx context.Context, request *QueryMessagesRequest, response *QueryMessagesResponse) error
}

type QuerySyncRequest struct{}

type QueryStateRequest struct {
	RoomID string
}

type QueryStateTypeRequest struct {
	RoomID    string
	EventType string
	StateKey  string
}

type QueryMessagesRequest struct {
	RoomID string
}

type QuerySyncResponse util.JSONResponse
type QueryStateResponse util.JSONResponse
type QueryStateTypeResponse util.JSONResponse
type QueryMessagesResponse util.JSONResponse

// QueryLatestEventsAndState implements SyncQueryAPI
func (h *httpSyncQueryAPI) QuerySync(
	ctx context.Context,
	request *QuerySyncRequest,
	response *QuerySyncResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QuerySync")
	defer span.Finish()

	apiURL := h.syncapiURL + SyncAPIQuerySyncPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryStateAfterEvents implements SyncQueryAPI
func (h *httpSyncQueryAPI) QueryState(
	ctx context.Context,
	request *QueryStateRequest,
	response *QueryStateResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryState")
	defer span.Finish()

	apiURL := h.syncapiURL + SyncAPIQueryStatePath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryEventsByID implements SyncQueryAPI
func (h *httpSyncQueryAPI) QueryStateType(
	ctx context.Context,
	request *QueryStateTypeRequest,
	response *QueryStateTypeResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryStateType")
	defer span.Finish()

	apiURL := h.syncapiURL + SyncAPIQueryStateTypePath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMembershipForUser implements SyncQueryAPI
func (h *httpSyncQueryAPI) QueryMessages(
	ctx context.Context,
	request *QueryMessagesRequest,
	response *QueryMessagesResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMessages")
	defer span.Finish()

	apiURL := h.syncapiURL + SyncAPIQueryMessagesPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
