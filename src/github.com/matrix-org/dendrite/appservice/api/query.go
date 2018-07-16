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
	"database/sql"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/appservice/types"
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

// UserIDExistsRequest is a request to an application service about whether a
// user ID exists
type UserIDExistsRequest struct {
	// UserID we want to lookup
	UserID string `json:"user_id"`
}

// UserIDExistsRequestAccessToken is a request to an application service
// about whether a user ID exists. Includes an access token
type UserIDExistsRequestAccessToken struct {
	// UserID we want to lookup
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
}

// UserIDExistsResponse is a response from an application service about
// whether a user ID exists
type UserIDExistsResponse struct {
	UserIDExists bool `json:"exists"`
}

// GetProtocolDefinitionRequest is a request to the appservice component asking
// for the definition of a single third party protocol
type GetProtocolDefinitionRequest struct {
	ProtocolID string `json:"protocol_definition"`
}

// GetProtocolDefinitionResponse is a response providing a protocol definition
// for the given protocol ID
type GetProtocolDefinitionResponse struct {
	ProtocolDefinition string `json:"protocol_definition"`
}

// GetAllProtocolDefinitionsRequest is a request to the appservice component
// asking for what third party protocols are known and their definitions
type GetAllProtocolDefinitionsRequest struct {
}

// GetAllProtocolDefinitionsResponse is a response containing all known third
// party IDs and their definitions
type GetAllProtocolDefinitionsResponse struct {
	Protocols types.ThirdPartyProtocols `json:"protocols"`
}

// AppServiceQueryAPI is used to query user and room alias data from application
// services
type AppServiceQueryAPI interface {
	// Check whether a room alias exists within any application service namespaces
	RoomAliasExists(
		ctx context.Context,
		req *RoomAliasExistsRequest,
		resp *RoomAliasExistsResponse,
	) error
	// Check whether a user ID exists within any application service namespaces
	UserIDExists(
		ctx context.Context,
		req *UserIDExistsRequest,
		resp *UserIDExistsResponse,
	) error
	// Get the definition of a given single third party protocol
	GetProtocolDefinition(
		ctx context.Context,
		req *GetProtocolDefinitionRequest,
		response *GetProtocolDefinitionResponse,
	) error
	// Get the definition of all known third party protocols
	GetAllProtocolDefinitions(
		ctx context.Context,
		req *GetAllProtocolDefinitionsRequest,
		response *GetAllProtocolDefinitionsResponse,
	) error
}

// RoomAliasExistsPath is the HTTP path for the RoomAliasExists API
const RoomAliasExistsPath = "/api/appservice/RoomAliasExists"

// GetProtocolDefinitionPath is the HTTP path for the GetProtocolDefinition API
const GetProtocolDefinitionPath = "/api/appservice/GetProtocolDefinition"

// GetAllProtocolDefinitionsPath is the HTTP path for the GetAllProtocolDefinitions API
const GetAllProtocolDefinitionsPath = "/api/appservice/GetAllProtocolDefinitions"

// AppServiceUserIDExistsPath is the HTTP path for the UserIDExists API
const AppServiceUserIDExistsPath = "/api/appservice/UserIDExists"

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

	apiURL := h.appserviceURL + RoomAliasExistsPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetProtocolDefinition implements AppServiceQueryAPI
func (h *httpAppServiceQueryAPI) GetProtocolDefinition(
	ctx context.Context,
	request *GetProtocolDefinitionRequest,
	response *GetProtocolDefinitionResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "appserviceGetProtocolDefinition")
	defer span.Finish()

	apiURL := h.appserviceURL + GetProtocolDefinitionPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetAllProtocolDefinitions implements AppServiceQueryAPI
func (h *httpAppServiceQueryAPI) GetAllProtocolDefinitions(
	ctx context.Context,
	request *GetAllProtocolDefinitionsRequest,
	response *GetAllProtocolDefinitionsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "appserviceGetAllProtocolDefinitions")
	defer span.Finish()

	apiURL := h.appserviceURL + GetAllProtocolDefinitionsPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// UserIDExists implements AppServiceQueryAPI
func (h *httpAppServiceQueryAPI) UserIDExists(
	ctx context.Context,
	request *UserIDExistsRequest,
	response *UserIDExistsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "appserviceUserIDExists")
	defer span.Finish()

	apiURL := h.appserviceURL + AppServiceUserIDExistsPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// RetreiveUserProfile is a wrapper that queries both the local database and
// application services for a given user's profile
func RetreiveUserProfile(
	ctx context.Context,
	userID string,
	asAPI AppServiceQueryAPI,
	accountDB *accounts.Database,
) (*authtypes.Profile, error) {
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	// Try to query the user from the local database
	profile, err := accountDB.GetProfileByLocalpart(ctx, localpart)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	} else if profile != nil {
		return profile, nil
	}

	// Query the appservice component for the existence of an AS user
	userReq := UserIDExistsRequest{UserID: userID}
	var userResp UserIDExistsResponse
	if err = asAPI.UserIDExists(ctx, &userReq, &userResp); err != nil {
		return nil, err
	}

	// If no user exists, return
	if !userResp.UserIDExists {
		return nil, errors.New("no known profile for given user ID")
	}

	// Try to query the user from the local database again
	profile, err = accountDB.GetProfileByLocalpart(ctx, localpart)
	if err != nil {
		return nil, err
	}

	// profile should not be nil at this point
	return profile, nil
}
