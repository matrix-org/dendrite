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

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	opentracing "github.com/opentracing/opentracing-go"
)

// SetRoomCanonicalAliasRequest is a request to SetRoomCanonicalAlias
type SetRoomCanonicalAliasRequest struct {
	// ID of the user setting the alias
	UserID string `json:"user_id"`
	// New alias for the room
	CanonicalAlias string `json:"alias"`
	// The room ID the alias is referring to
	RoomID string `json:"room_id"`
}

// SetRoomCanonicalAliasResponse is a response to SetRoomCanonicalAlias
type SetRoomCanonicalAliasResponse struct {
	// Does the canonical alias already belong to a room?
	CanonicalAliasExists bool `json:"canonical_alias_exists"`
}

// GetRoomIDForCanonicalAliasRequest is a request to GetRoomIDForCanonicalAlias
type GetRoomIDForCanonicalAliasRequest struct {
	// Canonical Alias we want to lookup
	CanonicalAlias string `json:"alias"`
}

// GetRoomIDForCanonicalAliasResponse is a response to GetRoomIDForCanonicalAlias
type GetRoomIDForCanonicalAliasResponse struct {
	// The room ID the canonical alias refers to
	RoomID string `json:"room_id"`
}

// GetCanonicalAliasForRoomIDRequest is a request to GetCanonicalAliasForRoomID
type GetCanonicalAliasForRoomIDRequest struct {
	// The room ID we want to find the canonical for
	RoomID string `json:"room_id"`
}

// GetCanonicalForRoomIDResponse is a response to GetCanonicalAliasForRoomID
type GetCanonicalAliasForRoomIDResponse struct {
	CanonicalAlias string `json:"alias"`
}

// GetCreatorIDForCanonicalAliasRequest is a request to GetCreatorIDForCanonicalAlias
type GetCreatorIDForCanonicalAliasRequest struct {
	// The alias we want to find the creator of
	CanonicalAlias string `json:"alias"`
}

// GetCreatorIDForCanonicalAliasResponse is a response to GetCreatorIDForCanonicalAlias
type GetCreatorIDForCanonicalAliasResponse struct {
	// The user ID of the canonical alias creator
	UserID string `json:"user_id"`
}

// RoomserverCanonicalAliasAPI is used to save, lookup or remove a room alias
type RoomserverCanonicalAliasAPI interface {
	// Set the room canonical alias
	SetRoomCanonicalAlias(
		ctx context.Context,
		req *SetRoomCanonicalAliasRequest,
		response *SetRoomCanonicalAliasResponse,
	) error

	// Get the room ID for an alias
	GetRoomIDForCanonicalAlias(
		ctx context.Context,
		req *GetRoomIDForCanonicalAliasRequest,
		response *GetRoomIDForCanonicalAliasResponse,
	) error

	// Get the canonical alias for a room ID
	GetCanonicalAliasForRoomID(
		ctx context.Context,
		req *GetCanonicalAliasForRoomIDRequest,
		response *GetCanonicalAliasForRoomIDResponse,
	) error

	// Get the user ID of the creator of a canonical alias
	GetCreatorIDForCanonicalAlias(
		ctx context.Context,
		req *GetCreatorIDForCanonicalAliasRequest,
		response *GetCreatorIDForCanonicalAliasResponse,
	) error
}

// RoomserverSetRoomCanonicalAliasPath is the HTTP path for the SetRoomCanonicalAlias API.
const RoomserverSetRoomCanonicalAliasPath = "/api/roomserver/setRoomCanonicalAlias"

// RoomserverGetRoomIDForCanonicalAliasPath is the HTTP path for the GetRoomIDForCanonicalAlias API.
const RoomserverGetRoomIDForCanonicalAliasPath = "/api/roomserver/GetRoomIDForCanonicalAlias"

// RoomserverGetCanonicalAliasForRoomIDPath is the HTTP path for the GetCanonicalAliasForRoomID API.
const RoomserverGetCanonicalAliasForRoomIDPath = "/api/roomserver/GetCanonicalAliasForRoomID"

// RoomserverGetCreatorIDForCanonicalAliasPath is the HTTP path for the GetCreatorIDForCanonicalAlias API.
const RoomserverGetCreatorIDForCanonicalAliasPath = "/api/roomserver/GetCreatorIDForCanonicalAlias"

// NewRoomserverCanonicalAliasAPIHTTP creates a RoomserverCanonicalAliasAPI implemented by talking to a HTTP POST API.
// If httpClient is nil then it uses the http.DefaultClient
func NewRoomserverCanonicalAliasAPIHTTP(roomserverURL string, httpClient *http.Client) RoomserverCanonicalAliasAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpRoomserverCanonicalAliasAPI{roomserverURL, httpClient}
}

type httpRoomserverCanonicalAliasAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

// SetRoomCanonicalAlias implements RoomserverCanonicalAliasAPI
func (h *httpRoomserverCanonicalAliasAPI) SetRoomCanonicalAlias(
	ctx context.Context,
	request *SetRoomCanonicalAliasRequest,
	response *SetRoomCanonicalAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "SetRoomCanonicalAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverSetRoomCanonicalAliasPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetRoomIDForCanonicalAlias implements RoomserverCanonicalAliasAPI
func (h *httpRoomserverCanonicalAliasAPI) GetRoomIDForCanonicalAlias(
	ctx context.Context,
	request *GetRoomIDForCanonicalAliasRequest,
	response *GetRoomIDForCanonicalAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetRoomIDForCanonicalAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetRoomIDForCanonicalAliasPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetCanonicalAliasForRoomID implements RoomserverCanonicalAliasAPI
func (h *httpRoomserverCanonicalAliasAPI) GetCanonicalAliasForRoomID(
	ctx context.Context,
	request *GetCanonicalAliasForRoomIDRequest,
	response *GetCanonicalAliasForRoomIDResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetCanonicalAliasForRoomID")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetCanonicalAliasForRoomIDPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetCreatorIDForAlias implements RoomserverCanonicalAliasAPI
func (h *httpRoomserverCanonicalAliasAPI) GetCreatorIDForCanonicalAlias(
	ctx context.Context,
	request *GetCreatorIDForCanonicalAliasRequest,
	response *GetCreatorIDForCanonicalAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetCreatorIDForCanonicalAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetCreatorIDForCanonicalAliasPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
