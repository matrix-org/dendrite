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
	"encoding/json"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrixserverlib"
)

// UserInternalAPI is the internal API for information about users and devices.
type UserInternalAPI interface {
	InputAccountData(ctx context.Context, req *InputAccountDataRequest, res *InputAccountDataResponse) error
	PerformAccountCreation(ctx context.Context, req *PerformAccountCreationRequest, res *PerformAccountCreationResponse) error
	PerformDeviceCreation(ctx context.Context, req *PerformDeviceCreationRequest, res *PerformDeviceCreationResponse) error
	QueryProfile(ctx context.Context, req *QueryProfileRequest, res *QueryProfileResponse) error
	QueryAccessToken(ctx context.Context, req *QueryAccessTokenRequest, res *QueryAccessTokenResponse) error
	QueryDevices(ctx context.Context, req *QueryDevicesRequest, res *QueryDevicesResponse) error
	QueryAccountData(ctx context.Context, req *QueryAccountDataRequest, res *QueryAccountDataResponse) error
	QueryDeviceInfos(ctx context.Context, req *QueryDeviceInfosRequest, res *QueryDeviceInfosResponse) error
	QuerySearchProfiles(ctx context.Context, req *QuerySearchProfilesRequest, res *QuerySearchProfilesResponse) error
}

// InputAccountDataRequest is the request for InputAccountData
type InputAccountDataRequest struct {
	UserID      string          // required: the user to set account data for
	RoomID      string          // optional: the room to associate the account data with
	DataType    string          // required: the data type of the data
	AccountData json.RawMessage // required: the message content
}

// InputAccountDataResponse is the response for InputAccountData
type InputAccountDataResponse struct {
}

// QueryDeviceInfosRequest is the request to QueryDeviceInfos
type QueryDeviceInfosRequest struct {
	DeviceIDs []string
}

// QueryDeviceInfosResponse is the response to QueryDeviceInfos
type QueryDeviceInfosResponse struct {
	DeviceInfo map[string]struct {
		DisplayName string
		UserID      string
	}
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

// QueryAccountDataRequest is the request for QueryAccountData
type QueryAccountDataRequest struct {
	UserID   string // required: the user to get account data for.
	RoomID   string // optional: the room ID, or global account data if not specified.
	DataType string // optional: the data type, or all types if not specified.
}

// QueryAccountDataResponse is the response for QueryAccountData
type QueryAccountDataResponse struct {
	GlobalAccountData map[string]json.RawMessage            // type -> data
	RoomAccountData   map[string]map[string]json.RawMessage // room -> type -> data
}

// QueryDevicesRequest is the request for QueryDevices
type QueryDevicesRequest struct {
	UserID string
}

// QueryDevicesResponse is the response for QueryDevices
type QueryDevicesResponse struct {
	UserExists bool
	Devices    []Device
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

// QuerySearchProfilesRequest is the request for QueryProfile
type QuerySearchProfilesRequest struct {
	// The search string to match
	SearchString string
	// How many results to return
	Limit int
}

// QuerySearchProfilesResponse is the response for QuerySearchProfilesRequest
type QuerySearchProfilesResponse struct {
	// Profiles matching the search
	Profiles []authtypes.Profile
}

// PerformAccountCreationRequest is the request for PerformAccountCreation
type PerformAccountCreationRequest struct {
	AccountType AccountType // Required: whether this is a guest or user account
	Localpart   string      // Required: The localpart for this account. Ignored if account type is guest.

	AppServiceID string // optional: the application service ID (not user ID) creating this account, if any.
	Password     string // optional: if missing then this account will be a passwordless account
	OnConflict   Conflict
}

// PerformAccountCreationResponse is the response for PerformAccountCreation
type PerformAccountCreationResponse struct {
	AccountCreated bool
	Account        *Account
}

// PerformDeviceCreationRequest is the request for PerformDeviceCreation
type PerformDeviceCreationRequest struct {
	Localpart   string
	AccessToken string // optional: if blank one will be made on your behalf
	// optional: if nil an ID is generated for you. If set, replaces any existing device session,
	// which will generate a new access token and invalidate the old one.
	DeviceID *string
	// optional: if nil no display name will be associated with this device.
	DeviceDisplayName *string
}

// PerformDeviceCreationResponse is the response for PerformDeviceCreation
type PerformDeviceCreationResponse struct {
	DeviceCreated bool
	Device        *Device
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

// Account represents a Matrix account on this home server.
type Account struct {
	UserID       string
	Localpart    string
	ServerName   gomatrixserverlib.ServerName
	AppServiceID string
	// TODO: Other flags like IsAdmin, IsGuest
	// TODO: Associations (e.g. with application services)
}

// ErrorForbidden is an error indicating that the supplied access token is forbidden
type ErrorForbidden struct {
	Message string
}

func (e *ErrorForbidden) Error() string {
	return "Forbidden: " + e.Message
}

// ErrorConflict is an error indicating that there was a conflict which resulted in the request being aborted.
type ErrorConflict struct {
	Message string
}

func (e *ErrorConflict) Error() string {
	return "Conflict: " + e.Message
}

// Conflict is an enum representing what to do when encountering conflicting when creating profiles/devices
type Conflict int

// AccountType is an enum representing the kind of account
type AccountType int

const (
	// ConflictUpdate will update matching records returning no error
	ConflictUpdate Conflict = 1
	// ConflictAbort will reject the request with ErrorConflict
	ConflictAbort Conflict = 2

	// AccountTypeUser indicates this is a user account
	AccountTypeUser AccountType = 1
	// AccountTypeGuest indicates this is a guest account
	AccountTypeGuest AccountType = 2
)
