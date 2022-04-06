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
	"errors"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
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
}

// RetrieveUserProfile is a wrapper that queries both the local database and
// application services for a given user's profile
// TODO: Remove this, it's called from federationapi and clientapi but is a pure function
func RetrieveUserProfile(
	ctx context.Context,
	userID string,
	asAPI AppServiceQueryAPI,
	profileAPI userapi.UserProfileAPI,
) (*authtypes.Profile, error) {
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	// Try to query the user from the local database
	res := &userapi.QueryProfileResponse{}
	err = profileAPI.QueryProfile(ctx, &userapi.QueryProfileRequest{UserID: userID}, res)
	if err != nil {
		return nil, err
	}
	profile := &authtypes.Profile{
		Localpart:   localpart,
		DisplayName: res.DisplayName,
		AvatarURL:   res.AvatarURL,
	}
	if res.UserExists {
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
	err = profileAPI.QueryProfile(ctx, &userapi.QueryProfileRequest{UserID: userID}, res)
	if err != nil {
		return nil, err
	}

	// profile should not be nil at this point
	return &authtypes.Profile{
		Localpart:   localpart,
		DisplayName: res.DisplayName,
		AvatarURL:   res.AvatarURL,
	}, nil
}
