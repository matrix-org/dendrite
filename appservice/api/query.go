// Copyright 2018-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

// Package api contains methods used by dendrite components in multi-process
// mode to send requests to the appservice component, typically in order to ask
// an application service for some information.
package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	userapi "github.com/element-hq/dendrite/userapi/api"
)

// AppServiceInternalAPI is used to query user and room alias data from application
// services
type AppServiceInternalAPI interface {
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

	Locations(ctx context.Context, req *LocationRequest, resp *LocationResponse) error
	User(ctx context.Context, request *UserRequest, response *UserResponse) error
	Protocols(ctx context.Context, req *ProtocolRequest, resp *ProtocolResponse) error
}

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

const (
	ASProtocolLegacyPath        = "/_matrix/app/unstable/thirdparty/protocol/"
	ASUserLegacyPath            = "/_matrix/app/unstable/thirdparty/user"
	ASLocationLegacyPath        = "/_matrix/app/unstable/thirdparty/location"
	ASRoomAliasExistsLegacyPath = "/rooms/"
	ASUserExistsLegacyPath      = "/users/"

	ASProtocolPath        = "/_matrix/app/v1/thirdparty/protocol/"
	ASUserPath            = "/_matrix/app/v1/thirdparty/user"
	ASLocationPath        = "/_matrix/app/v1/thirdparty/location"
	ASRoomAliasExistsPath = "/_matrix/app/v1/rooms/"
	ASUserExistsPath      = "/_matrix/app/v1/users/"
)

type ProtocolRequest struct {
	Protocol string `json:"protocol,omitempty"`
}

type ProtocolResponse struct {
	Protocols map[string]ASProtocolResponse `json:"protocols"`
	Exists    bool                          `json:"exists"`
}

type ASProtocolResponse struct {
	FieldTypes     map[string]FieldType `json:"field_types,omitempty"` // NOTSPEC: field_types is required by the spec
	Icon           string               `json:"icon"`
	Instances      []ProtocolInstance   `json:"instances"`
	LocationFields []string             `json:"location_fields"`
	UserFields     []string             `json:"user_fields"`
}

type FieldType struct {
	Placeholder string `json:"placeholder"`
	Regexp      string `json:"regexp"`
}

type ProtocolInstance struct {
	Description string          `json:"desc"`
	Icon        string          `json:"icon,omitempty"`
	NetworkID   string          `json:"network_id,omitempty"` // NOTSPEC: network_id is required by the spec
	Fields      json.RawMessage `json:"fields,omitempty"`     // NOTSPEC: fields is required by the spec
}

type UserRequest struct {
	Protocol string `json:"protocol"`
	Params   string `json:"params"`
}

type UserResponse struct {
	Users  []ASUserResponse `json:"users,omitempty"`
	Exists bool             `json:"exists,omitempty"`
}

type ASUserResponse struct {
	Protocol string          `json:"protocol"`
	UserID   string          `json:"userid"`
	Fields   json.RawMessage `json:"fields"`
}

type LocationRequest struct {
	Protocol string `json:"protocol"`
	Params   string `json:"params"`
}

type LocationResponse struct {
	Locations []ASLocationResponse `json:"locations,omitempty"`
	Exists    bool                 `json:"exists,omitempty"`
}

type ASLocationResponse struct {
	Alias    string          `json:"alias"`
	Protocol string          `json:"protocol"`
	Fields   json.RawMessage `json:"fields"`
}

// ErrProfileNotExists is returned when trying to lookup a user's profile that
// doesn't exist locally.
var ErrProfileNotExists = errors.New("no known profile for given user ID")

// RetrieveUserProfile is a wrapper that queries both the local database and
// application services for a given user's profile
// TODO: Remove this, it's called from federationapi and clientapi but is a pure function
func RetrieveUserProfile(
	ctx context.Context,
	userID string,
	asAPI AppServiceInternalAPI,
	profileAPI userapi.ProfileAPI,
) (*authtypes.Profile, error) {
	// Try to query the user from the local database
	profile, err := profileAPI.QueryProfile(ctx, userID)
	if err == nil {
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
		return nil, ErrProfileNotExists
	}

	// Try to query the user from the local database again
	profile, err = profileAPI.QueryProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	// profile should not be nil at this point
	return profile, nil
}
