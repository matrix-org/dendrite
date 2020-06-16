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

import "context"

// UserInternalAPI is the internal API for information about users and devices.
type UserInternalAPI interface {
	QueryProfile(ctx context.Context, req *QueryProfileRequest, res *QueryProfileResponse) error
	QueryAccessToken(ctx context.Context, req *QueryAccessTokenRequest, res *QueryAccessTokenResponse) error
}

// QueryAccessTokenRequest is the request for QueryAccessToken
type QueryAccessTokenRequest struct {
	AccessToken string
	// optional user ID, valid only if the token is an appservice.
	// https://matrix.org/docs/spec/application_service/r0.1.2#using-sync-and-events
	AppServiceUserID string
}

// QueryAccessTokenResponse is the response for QueryAccessToken
type QueryAccessTokenResponse struct {
	Device *Device
	Err    error // e.g ErrorForbidden
}

// QueryProfileRequest is the request for QueryProfile
type QueryProfileRequest struct {
	// The user ID to query
	UserID string
}

// QueryProfileResponse is the response for QueryProfile
type QueryProfileResponse struct {
	// True if the user exists. Querying for a profile does not create them.
	UserExists bool
	// The current display name if set.
	DisplayName string
	// The current avatar URL if set.
	AvatarURL string
}

// Device represents a client's device (mobile, web, etc)
type Device struct {
	ID     string
	UserID string
	// The access_token granted to this device.
	// This uniquely identifies the device from all other devices and clients.
	AccessToken string
	// The unique ID of the session identified by the access token.
	// Can be used as a secure substitution in places where data needs to be
	// associated with access tokens.
	SessionID int64
	// TODO: display name, last used timestamp, keys, etc
	DisplayName string
}

// ErrorForbidden is an error indicating that the supplied access token is forbidden
type ErrorForbidden struct {
	Message string
}

func (e *ErrorForbidden) Error() string {
	return "Forbidden: " + e.Message
}
