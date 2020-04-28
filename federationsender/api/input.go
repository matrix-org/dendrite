// Copyright 2017 Vector Creations Ltd
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

// Package api provides the types that are used to communicate with the roomserver.
package api

import (
	"context"
	"errors"
	"net/http"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	opentracing "github.com/opentracing/opentracing-go"
)

// FederationSenderInputAPI is used to write events to the room server.
type FederationSenderInputAPI interface {
	InputJoinRequest(
		ctx context.Context,
		request *InputJoinRequest,
		response *InputJoinResponse,
	) error
	InputLeaveRequest(
		ctx context.Context,
		request *InputLeaveRequest,
		response *InputLeaveResponse,
	) error
}

const RoomserverInputJoinRequestPath = "/api/roomserver/inputJoinRequest"
const RoomserverInputLeaveRequestPath = "/api/roomserver/inputLeaveRequest"

type InputJoinRequest struct {
	RoomID string `json:"room_id"`
}

type InputJoinResponse struct {
}

type InputLeaveRequest struct {
	RoomID string `json:"room_id"`
}

type InputLeaveResponse struct {
}

// NewFederationSenderInputAPIHTTP creates a FederationSenderInputAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewFederationSenderInputAPIHTTP(roomserverURL string, httpClient *http.Client) (FederationSenderInputAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewFederationSenderInputAPIHTTP: httpClient is <nil>")
	}
	return &httpFederationSenderInputAPI{roomserverURL, httpClient}, nil
}

type httpFederationSenderInputAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

func (h *httpFederationSenderInputAPI) InputJoinRequest(
	ctx context.Context,
	request *InputJoinRequest,
	response *InputJoinResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputRoomEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverInputJoinRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpFederationSenderInputAPI) InputLeaveRequest(
	ctx context.Context,
	request *InputLeaveRequest,
	response *InputLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputRoomEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverInputLeaveRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
