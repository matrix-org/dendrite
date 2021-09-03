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
	PerformPasswordUpdate(ctx context.Context, req *PerformPasswordUpdateRequest, res *PerformPasswordUpdateResponse) error
	PerformDeviceCreation(ctx context.Context, req *PerformDeviceCreationRequest, res *PerformDeviceCreationResponse) error
	PerformDeviceDeletion(ctx context.Context, req *PerformDeviceDeletionRequest, res *PerformDeviceDeletionResponse) error
	PerformLastSeenUpdate(ctx context.Context, req *PerformLastSeenUpdateRequest, res *PerformLastSeenUpdateResponse) error
	PerformDeviceUpdate(ctx context.Context, req *PerformDeviceUpdateRequest, res *PerformDeviceUpdateResponse) error
	PerformAccountDeactivation(ctx context.Context, req *PerformAccountDeactivationRequest, res *PerformAccountDeactivationResponse) error
	PerformOpenIDTokenCreation(ctx context.Context, req *PerformOpenIDTokenCreationRequest, res *PerformOpenIDTokenCreationResponse) error
	PerformKeyBackup(ctx context.Context, req *PerformKeyBackupRequest, res *PerformKeyBackupResponse)
	QueryKeyBackup(ctx context.Context, req *QueryKeyBackupRequest, res *QueryKeyBackupResponse)
	QueryProfile(ctx context.Context, req *QueryProfileRequest, res *QueryProfileResponse) error
	QueryAccessToken(ctx context.Context, req *QueryAccessTokenRequest, res *QueryAccessTokenResponse) error
	QueryDevices(ctx context.Context, req *QueryDevicesRequest, res *QueryDevicesResponse) error
	QueryAccountData(ctx context.Context, req *QueryAccountDataRequest, res *QueryAccountDataResponse) error
	QueryDeviceInfos(ctx context.Context, req *QueryDeviceInfosRequest, res *QueryDeviceInfosResponse) error
	QuerySearchProfiles(ctx context.Context, req *QuerySearchProfilesRequest, res *QuerySearchProfilesResponse) error
	QueryOpenIDToken(ctx context.Context, req *QueryOpenIDTokenRequest, res *QueryOpenIDTokenResponse) error
}

type PerformKeyBackupRequest struct {
	UserID       string
	Version      string // optional if modifying a key backup
	AuthData     json.RawMessage
	Algorithm    string
	DeleteBackup bool // if true will delete the backup based on 'Version'.

	// The keys to upload, if any. If blank, creates/updates/deletes key version metadata only.
	Keys struct {
		Rooms map[string]struct {
			Sessions map[string]KeyBackupSession `json:"sessions"`
		} `json:"rooms"`
	}
}

// KeyBackupData in https://spec.matrix.org/unstable/client-server-api/#get_matrixclientr0room_keyskeysroomidsessionid
type KeyBackupSession struct {
	FirstMessageIndex int             `json:"first_message_index"`
	ForwardedCount    int             `json:"forwarded_count"`
	IsVerified        bool            `json:"is_verified"`
	SessionData       json.RawMessage `json:"session_data"`
}

func (a *KeyBackupSession) ShouldReplaceRoomKey(newKey *KeyBackupSession) bool {
	// https://spec.matrix.org/unstable/client-server-api/#backup-algorithm-mmegolm_backupv1curve25519-aes-sha2
	// "if the keys have different values for is_verified, then it will keep the key that has is_verified set to true"
	if newKey.IsVerified && !a.IsVerified {
		return true
	} else if newKey.FirstMessageIndex < a.FirstMessageIndex {
		// "if they have the same values for is_verified, then it will keep the key with a lower first_message_index"
		return true
	} else if newKey.ForwardedCount < a.ForwardedCount {
		// "and finally, is is_verified and first_message_index are equal, then it will keep the key with a lower forwarded_count"
		return true
	}
	return false
}

// Internal KeyBackupData for passing to/from the storage layer
type InternalKeyBackupSession struct {
	KeyBackupSession
	RoomID    string
	SessionID string
}

type PerformKeyBackupResponse struct {
	Error    string // set if there was a problem performing the request
	BadInput bool   // if set, the Error was due to bad input (HTTP 400)

	Exists  bool   // set to true if the Version exists
	Version string // the newly created version

	KeyCount int64  // only set if Keys were given in the request
	KeyETag  string // only set if Keys were given in the request
}

type QueryKeyBackupRequest struct {
	UserID  string
	Version string // the version to query, if blank it means the latest

	ReturnKeys       bool   // whether to return keys in the backup response or just the metadata
	KeysForRoomID    string // optional string to return keys which belong to this room
	KeysForSessionID string // optional string to return keys which belong to this (room, session)
}

type QueryKeyBackupResponse struct {
	Error  string
	Exists bool

	Algorithm string          `json:"algorithm"`
	AuthData  json.RawMessage `json:"auth_data"`
	Count     int64           `json:"count"`
	ETag      string          `json:"etag"`
	Version   string          `json:"version"`

	Keys map[string]map[string]KeyBackupSession // the keys if ReturnKeys=true
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

type PerformDeviceUpdateRequest struct {
	RequestingUserID string
	DeviceID         string
	DisplayName      *string
}
type PerformDeviceUpdateResponse struct {
	DeviceExists bool
	Forbidden    bool
}

type PerformDeviceDeletionRequest struct {
	UserID string
	// The devices to delete. An empty slice means delete all devices.
	DeviceIDs []string
	// The requesting device ID to exclude from deletion. This is needed
	// so that a password change doesn't cause that client to be logged
	// out. Only specify when DeviceIDs is empty.
	ExceptDeviceID string
}

type PerformDeviceDeletionResponse struct {
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

// PerformAccountCreationRequest is the request for PerformAccountCreation
type PerformPasswordUpdateRequest struct {
	Localpart string // Required: The localpart for this account.
	Password  string // Required: The new password to set.
}

// PerformAccountCreationResponse is the response for PerformAccountCreation
type PerformPasswordUpdateResponse struct {
	PasswordUpdated bool
	Account         *Account
}

// PerformLastSeenUpdateRequest is the request for PerformLastSeenUpdate.
type PerformLastSeenUpdateRequest struct {
	UserID     string
	DeviceID   string
	RemoteAddr string
}

// PerformLastSeenUpdateResponse is the response for PerformLastSeenUpdate.
type PerformLastSeenUpdateResponse struct {
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
	// IP address of this device
	IPAddr string
	// Useragent for this device
	UserAgent string
}

// PerformDeviceCreationResponse is the response for PerformDeviceCreation
type PerformDeviceCreationResponse struct {
	DeviceCreated bool
	Device        *Device
}

// PerformAccountDeactivationRequest is the request for PerformAccountDeactivation
type PerformAccountDeactivationRequest struct {
	Localpart string
}

// PerformAccountDeactivationResponse is the response for PerformAccountDeactivation
type PerformAccountDeactivationResponse struct {
	AccountDeactivated bool
}

// PerformOpenIDTokenCreationRequest is the request for PerformOpenIDTokenCreation
type PerformOpenIDTokenCreationRequest struct {
	UserID string
}

// PerformOpenIDTokenCreationResponse is the response for PerformOpenIDTokenCreation
type PerformOpenIDTokenCreationResponse struct {
	Token OpenIDToken
}

// QueryOpenIDTokenRequest is the request for QueryOpenIDToken
type QueryOpenIDTokenRequest struct {
	Token string
}

// QueryOpenIDTokenResponse is the response for QueryOpenIDToken
type QueryOpenIDTokenResponse struct {
	Sub         string // The Matrix User ID that generated the token
	ExpiresAtMS int64
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
	SessionID   int64
	DisplayName string
	LastSeenTS  int64
	LastSeenIP  string
	UserAgent   string
	// If the device is for an appservice user,
	// this is the appservice ID.
	AppserviceID string
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

// OpenIDToken represents an OpenID token
type OpenIDToken struct {
	Token       string
	UserID      string
	ExpiresAtMS int64
}

// OpenIDTokenInfo represents the attributes associated with an issued OpenID token
type OpenIDTokenAttributes struct {
	UserID      string
	ExpiresAtMS int64
}

// UserInfo is for returning information about the user an OpenID token was issued for
type UserInfo struct {
	Sub string // The Matrix user's ID who generated the token
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
