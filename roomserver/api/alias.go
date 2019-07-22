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

package api

import (
	"context"
	"net/http"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	opentracing "github.com/opentracing/opentracing-go"
)

// SetRoomAliasRequest is a request to SetRoomAlias
type SetRoomAliasRequest struct {
	// ID of the user setting the alias
	UserID string `json:"user_id"`
	// New alias for the room
	Alias string `json:"alias"`
	// The room ID the alias is referring to
	RoomID string `json:"room_id"`
}

// SetRoomAliasResponse is a response to SetRoomAlias
type SetRoomAliasResponse struct {
	// Does the alias already refer to a room?
	AliasExists bool `json:"alias_exists"`
}

// GetRoomIDForAliasRequest is a request to GetRoomIDForAlias
type GetRoomIDForAliasRequest struct {
	// Alias we want to lookup
	Alias string `json:"alias"`
}

// GetRoomIDForAliasResponse is a response to GetRoomIDForAlias
type GetRoomIDForAliasResponse struct {
	// The room ID the alias refers to
	RoomID string `json:"room_id"`
}

// GetAliasesForRoomIDRequest is a request to GetAliasesForRoomID
type GetAliasesForRoomIDRequest struct {
	// The room ID we want to find aliases for
	RoomID string `json:"room_id"`
}

// GetAliasesForRoomIDResponse is a response to GetAliasesForRoomID
type GetAliasesForRoomIDResponse struct {
	// The aliases the alias refers to
	Aliases []string `json:"aliases"`
}

// RemoveRoomAliasRequest is a request to RemoveRoomAlias
type RemoveRoomAliasRequest struct {
	// ID of the user removing the alias
	UserID string `json:"user_id"`
	// The room alias to remove
	Alias string `json:"alias"`
}

// RemoveRoomAliasResponse is a response to RemoveRoomAlias
type RemoveRoomAliasResponse struct{}

// RoomserverAliasAPI is used to save, lookup or remove a room alias
type RoomserverAliasAPI interface {
	// Set a room alias
	SetRoomAlias(
		ctx context.Context,
		req *SetRoomAliasRequest,
		response *SetRoomAliasResponse,
	) error

	// Get the room ID for an alias
	GetRoomIDForAlias(
		ctx context.Context,
		req *GetRoomIDForAliasRequest,
		response *GetRoomIDForAliasResponse,
	) error

	// Get all known aliases for a room ID
	GetAliasesForRoomID(
		ctx context.Context,
		req *GetAliasesForRoomIDRequest,
		response *GetAliasesForRoomIDResponse,
	) error

	// Remove a room alias
	RemoveRoomAlias(
		ctx context.Context,
		req *RemoveRoomAliasRequest,
		response *RemoveRoomAliasResponse,
	) error
}

// RoomserverSetRoomAliasPath is the HTTP path for the SetRoomAlias API.
const RoomserverSetRoomAliasPath = "/api/roomserver/setRoomAlias"

// RoomserverGetRoomIDForAliasPath is the HTTP path for the GetRoomIDForAlias API.
const RoomserverGetRoomIDForAliasPath = "/api/roomserver/GetRoomIDForAlias"

// RoomserverGetAliasesForRoomIDPath is the HTTP path for the GetAliasesForRoomID API.
const RoomserverGetAliasesForRoomIDPath = "/api/roomserver/GetAliasesForRoomID"

// RoomserverRemoveRoomAliasPath is the HTTP path for the RemoveRoomAlias API.
const RoomserverRemoveRoomAliasPath = "/api/roomserver/removeRoomAlias"

// NewRoomserverAliasAPIHTTP creates a RoomserverAliasAPI implemented by talking to a HTTP POST API.
// If httpClient is nil then it uses the http.DefaultClient
func NewRoomserverAliasAPIHTTP(roomserverURL string, httpClient *http.Client) RoomserverAliasAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpRoomserverAliasAPI{roomserverURL, httpClient}
}

type httpRoomserverAliasAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

// SetRoomAlias implements RoomserverAliasAPI
func (h *httpRoomserverAliasAPI) SetRoomAlias(
	ctx context.Context,
	request *SetRoomAliasRequest,
	response *SetRoomAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "SetRoomAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverSetRoomAliasPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetRoomIDForAlias implements RoomserverAliasAPI
func (h *httpRoomserverAliasAPI) GetRoomIDForAlias(
	ctx context.Context,
	request *GetRoomIDForAliasRequest,
	response *GetRoomIDForAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetRoomIDForAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetRoomIDForAliasPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetAliasesForRoomID implements RoomserverAliasAPI
func (h *httpRoomserverAliasAPI) GetAliasesForRoomID(
	ctx context.Context,
	request *GetAliasesForRoomIDRequest,
	response *GetAliasesForRoomIDResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetAliasesForRoomID")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetAliasesForRoomIDPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// RemoveRoomAlias implements RoomserverAliasAPI
func (h *httpRoomserverAliasAPI) RemoveRoomAlias(
	ctx context.Context,
	request *RemoveRoomAliasRequest,
	response *RemoveRoomAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RemoveRoomAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverRemoveRoomAliasPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
