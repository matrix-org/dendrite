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
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/userapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/pushrules"
)

// UserInternalAPI is the internal API for information about users and devices.
type UserInternalAPI interface {
	SyncUserAPI
	ClientUserAPI
	FederationUserAPI

	QuerySearchProfilesAPI // used by p2p demos
	QueryAccountByLocalpart(ctx context.Context, req *QueryAccountByLocalpartRequest, res *QueryAccountByLocalpartResponse) (err error)
}

// api functions required by the appservice api
type AppserviceUserAPI interface {
	PerformAccountCreation(ctx context.Context, req *PerformAccountCreationRequest, res *PerformAccountCreationResponse) error
	PerformDeviceCreation(ctx context.Context, req *PerformDeviceCreationRequest, res *PerformDeviceCreationResponse) error
}

type RoomserverUserAPI interface {
	QueryAccountData(ctx context.Context, req *QueryAccountDataRequest, res *QueryAccountDataResponse) error
	QueryAccountByLocalpart(ctx context.Context, req *QueryAccountByLocalpartRequest, res *QueryAccountByLocalpartResponse) (err error)
}

// api functions required by the media api
type MediaUserAPI interface {
	QueryAcccessTokenAPI
}

// api functions required by the federation api
type FederationUserAPI interface {
	UploadDeviceKeysAPI
	QueryOpenIDToken(ctx context.Context, req *QueryOpenIDTokenRequest, res *QueryOpenIDTokenResponse) error
	QueryProfile(ctx context.Context, userID string) (*authtypes.Profile, error)
	QueryDevices(ctx context.Context, req *QueryDevicesRequest, res *QueryDevicesResponse) error
	QueryKeys(ctx context.Context, req *QueryKeysRequest, res *QueryKeysResponse) error
	QuerySignatures(ctx context.Context, req *QuerySignaturesRequest, res *QuerySignaturesResponse) error
	QueryDeviceMessages(ctx context.Context, req *QueryDeviceMessagesRequest, res *QueryDeviceMessagesResponse) error
	PerformClaimKeys(ctx context.Context, req *PerformClaimKeysRequest, res *PerformClaimKeysResponse) error
}

// api functions required by the sync api
type SyncUserAPI interface {
	QueryAcccessTokenAPI
	SyncKeyAPI
	QueryAccountData(ctx context.Context, req *QueryAccountDataRequest, res *QueryAccountDataResponse) error
	PerformLastSeenUpdate(ctx context.Context, req *PerformLastSeenUpdateRequest, res *PerformLastSeenUpdateResponse) error
	PerformDeviceUpdate(ctx context.Context, req *PerformDeviceUpdateRequest, res *PerformDeviceUpdateResponse) error
	QueryDevices(ctx context.Context, req *QueryDevicesRequest, res *QueryDevicesResponse) error
	QueryDeviceInfos(ctx context.Context, req *QueryDeviceInfosRequest, res *QueryDeviceInfosResponse) error
}

// api functions required by the client api
type ClientUserAPI interface {
	QueryAcccessTokenAPI
	LoginTokenInternalAPI
	UserLoginAPI
	ClientKeyAPI
	ProfileAPI
	QueryNumericLocalpart(ctx context.Context, req *QueryNumericLocalpartRequest, res *QueryNumericLocalpartResponse) error
	QueryDevices(ctx context.Context, req *QueryDevicesRequest, res *QueryDevicesResponse) error
	QueryAccountData(ctx context.Context, req *QueryAccountDataRequest, res *QueryAccountDataResponse) error
	QueryPushers(ctx context.Context, req *QueryPushersRequest, res *QueryPushersResponse) error
	QueryPushRules(ctx context.Context, userID string) (*pushrules.AccountRuleSets, error)
	QueryAccountAvailability(ctx context.Context, req *QueryAccountAvailabilityRequest, res *QueryAccountAvailabilityResponse) error
	PerformAccountCreation(ctx context.Context, req *PerformAccountCreationRequest, res *PerformAccountCreationResponse) error
	PerformDeviceCreation(ctx context.Context, req *PerformDeviceCreationRequest, res *PerformDeviceCreationResponse) error
	PerformDeviceUpdate(ctx context.Context, req *PerformDeviceUpdateRequest, res *PerformDeviceUpdateResponse) error
	PerformDeviceDeletion(ctx context.Context, req *PerformDeviceDeletionRequest, res *PerformDeviceDeletionResponse) error
	PerformPasswordUpdate(ctx context.Context, req *PerformPasswordUpdateRequest, res *PerformPasswordUpdateResponse) error
	PerformPusherDeletion(ctx context.Context, req *PerformPusherDeletionRequest, res *struct{}) error
	PerformPusherSet(ctx context.Context, req *PerformPusherSetRequest, res *struct{}) error
	PerformPushRulesPut(ctx context.Context, userID string, ruleSets *pushrules.AccountRuleSets) error
	PerformAccountDeactivation(ctx context.Context, req *PerformAccountDeactivationRequest, res *PerformAccountDeactivationResponse) error
	PerformOpenIDTokenCreation(ctx context.Context, req *PerformOpenIDTokenCreationRequest, res *PerformOpenIDTokenCreationResponse) error
	QueryNotifications(ctx context.Context, req *QueryNotificationsRequest, res *QueryNotificationsResponse) error
	InputAccountData(ctx context.Context, req *InputAccountDataRequest, res *InputAccountDataResponse) error
	PerformKeyBackup(ctx context.Context, req *PerformKeyBackupRequest, res *PerformKeyBackupResponse) error
	QueryKeyBackup(ctx context.Context, req *QueryKeyBackupRequest, res *QueryKeyBackupResponse) error

	QueryThreePIDsForLocalpart(ctx context.Context, req *QueryThreePIDsForLocalpartRequest, res *QueryThreePIDsForLocalpartResponse) error
	QueryLocalpartForThreePID(ctx context.Context, req *QueryLocalpartForThreePIDRequest, res *QueryLocalpartForThreePIDResponse) error
	PerformForgetThreePID(ctx context.Context, req *PerformForgetThreePIDRequest, res *struct{}) error
	PerformSaveThreePIDAssociation(ctx context.Context, req *PerformSaveThreePIDAssociationRequest, res *struct{}) error
}

type ProfileAPI interface {
	QueryProfile(ctx context.Context, userID string) (*authtypes.Profile, error)
	SetAvatarURL(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, avatarURL string) (*authtypes.Profile, bool, error)
	SetDisplayName(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, displayName string) (*authtypes.Profile, bool, error)
}

// custom api functions required by pinecone / p2p demos
type QuerySearchProfilesAPI interface {
	QuerySearchProfiles(ctx context.Context, req *QuerySearchProfilesRequest, res *QuerySearchProfilesResponse) error
}

// common function for creating authenticated endpoints (used in client/media/sync api)
type QueryAcccessTokenAPI interface {
	QueryAccessToken(ctx context.Context, req *QueryAccessTokenRequest, res *QueryAccessTokenResponse) error
}

type UserLoginAPI interface {
	QueryAccountByPassword(ctx context.Context, req *QueryAccountByPasswordRequest, res *QueryAccountByPasswordResponse) error
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
	Err    string // e.g ErrorForbidden
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
	AccountType AccountType                  // Required: whether this is a guest or user account
	Localpart   string                       // Required: The localpart for this account. Ignored if account type is guest.
	ServerName  gomatrixserverlib.ServerName // optional: if not specified, default server name used instead

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
	Localpart     string                       // Required: The localpart for this account.
	ServerName    gomatrixserverlib.ServerName // Required: The domain for this account.
	Password      string                       // Required: The new password to set.
	LogoutDevices bool                         // Optional: Whether to log out all user devices.
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
	UserAgent  string
}

// PerformLastSeenUpdateResponse is the response for PerformLastSeenUpdate.
type PerformLastSeenUpdateResponse struct {
}

// PerformDeviceCreationRequest is the request for PerformDeviceCreation
type PerformDeviceCreationRequest struct {
	Localpart   string
	ServerName  gomatrixserverlib.ServerName // optional: if blank, default server name used
	AccessToken string                       // optional: if blank one will be made on your behalf
	// optional: if nil an ID is generated for you. If set, replaces any existing device session,
	// which will generate a new access token and invalidate the old one.
	DeviceID *string
	// optional: if nil no display name will be associated with this device.
	DeviceDisplayName *string
	// IP address of this device
	IPAddr string
	// Useragent for this device
	UserAgent string
	// NoDeviceListUpdate determines whether we should avoid sending a device list
	// update for this account. Generally the only reason to do this is if the account
	// is an appservice account.
	NoDeviceListUpdate bool
}

// PerformDeviceCreationResponse is the response for PerformDeviceCreation
type PerformDeviceCreationResponse struct {
	DeviceCreated bool
	Device        *Device
}

// PerformAccountDeactivationRequest is the request for PerformAccountDeactivation
type PerformAccountDeactivationRequest struct {
	Localpart  string
	ServerName gomatrixserverlib.ServerName // optional: if blank, default server name used
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
	AccountType  AccountType
}

func (d *Device) UserDomain() gomatrixserverlib.ServerName {
	_, domain, err := gomatrixserverlib.SplitID('@', d.UserID)
	if err != nil {
		// This really is catastrophic because it means that someone
		// managed to forge a malformed user ID for a device during
		// login.
		// TODO: Is there a better way to deal with this than panic?
		panic(err)
	}
	return domain
}

// Account represents a Matrix account on this home server.
type Account struct {
	UserID       string
	Localpart    string
	ServerName   gomatrixserverlib.ServerName
	AppServiceID string
	AccountType  AccountType
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
	// AccountTypeAdmin indicates this is an admin account
	AccountTypeAdmin AccountType = 3
	// AccountTypeAppService indicates this is an appservice account
	AccountTypeAppService AccountType = 4
)

type QueryPushersRequest struct {
	Localpart  string
	ServerName gomatrixserverlib.ServerName
}

type QueryPushersResponse struct {
	Pushers []Pusher `json:"pushers"`
}

type PerformPusherSetRequest struct {
	Pusher     // Anonymous field because that's how clientapi unmarshals it.
	Localpart  string
	ServerName gomatrixserverlib.ServerName
	Append     bool `json:"append"`
}

type PerformPusherDeletionRequest struct {
	Localpart  string
	ServerName gomatrixserverlib.ServerName
	SessionID  int64
}

// Pusher represents a push notification subscriber
type Pusher struct {
	SessionID         int64                  `json:"session_id,omitempty"`
	PushKey           string                 `json:"pushkey"`
	PushKeyTS         int64                  `json:"pushkey_ts,omitempty"`
	Kind              PusherKind             `json:"kind"`
	AppID             string                 `json:"app_id"`
	AppDisplayName    string                 `json:"app_display_name"`
	DeviceDisplayName string                 `json:"device_display_name"`
	ProfileTag        string                 `json:"profile_tag"`
	Language          string                 `json:"lang"`
	Data              map[string]interface{} `json:"data"`
}

type PusherKind string

const (
	EmailKind PusherKind = "email"
	HTTPKind  PusherKind = "http"
)

type QueryNotificationsRequest struct {
	Localpart  string                       `json:"localpart"`   // Required.
	ServerName gomatrixserverlib.ServerName `json:"server_name"` // Required.
	From       string                       `json:"from,omitempty"`
	Limit      int                          `json:"limit,omitempty"`
	Only       string                       `json:"only,omitempty"`
}

type QueryNotificationsResponse struct {
	NextToken     string          `json:"next_token"`
	Notifications []*Notification `json:"notifications"` // Required.
}

type Notification struct {
	Actions    []*pushrules.Action         `json:"actions"`     // Required.
	Event      synctypes.ClientEvent       `json:"event"`       // Required.
	ProfileTag string                      `json:"profile_tag"` // Required by Sytest, but actually optional.
	Read       bool                        `json:"read"`        // Required.
	RoomID     string                      `json:"room_id"`     // Required.
	TS         gomatrixserverlib.Timestamp `json:"ts"`          // Required.
}

type QueryNumericLocalpartRequest struct {
	ServerName gomatrixserverlib.ServerName
}

type QueryNumericLocalpartResponse struct {
	ID int64
}

type QueryAccountAvailabilityRequest struct {
	Localpart  string
	ServerName gomatrixserverlib.ServerName
}

type QueryAccountAvailabilityResponse struct {
	Available bool
}

type QueryAccountByPasswordRequest struct {
	Localpart         string
	ServerName        gomatrixserverlib.ServerName
	PlaintextPassword string
}

type QueryAccountByPasswordResponse struct {
	Account *Account
	Exists  bool
}

type QueryLocalpartForThreePIDRequest struct {
	ThreePID, Medium string
}

type QueryLocalpartForThreePIDResponse struct {
	Localpart  string
	ServerName gomatrixserverlib.ServerName
}

type QueryThreePIDsForLocalpartRequest struct {
	Localpart  string
	ServerName gomatrixserverlib.ServerName
}

type QueryThreePIDsForLocalpartResponse struct {
	ThreePIDs []authtypes.ThreePID
}

type PerformForgetThreePIDRequest QueryLocalpartForThreePIDRequest

type PerformSaveThreePIDAssociationRequest struct {
	ThreePID   string
	Localpart  string
	ServerName gomatrixserverlib.ServerName
	Medium     string
}

type QueryAccountByLocalpartRequest struct {
	Localpart  string
	ServerName gomatrixserverlib.ServerName
}

type QueryAccountByLocalpartResponse struct {
	Account *Account
}

// API functions required by the clientapi
type ClientKeyAPI interface {
	UploadDeviceKeysAPI
	QueryKeys(ctx context.Context, req *QueryKeysRequest, res *QueryKeysResponse) error
	PerformUploadKeys(ctx context.Context, req *PerformUploadKeysRequest, res *PerformUploadKeysResponse) error

	PerformUploadDeviceSignatures(ctx context.Context, req *PerformUploadDeviceSignaturesRequest, res *PerformUploadDeviceSignaturesResponse) error
	// PerformClaimKeys claims one-time keys for use in pre-key messages
	PerformClaimKeys(ctx context.Context, req *PerformClaimKeysRequest, res *PerformClaimKeysResponse) error
	PerformMarkAsStaleIfNeeded(ctx context.Context, req *PerformMarkAsStaleRequest, res *struct{}) error
}

type UploadDeviceKeysAPI interface {
	PerformUploadDeviceKeys(ctx context.Context, req *PerformUploadDeviceKeysRequest, res *PerformUploadDeviceKeysResponse) error
}

// API functions required by the syncapi
type SyncKeyAPI interface {
	QueryKeyChanges(ctx context.Context, req *QueryKeyChangesRequest, res *QueryKeyChangesResponse) error
	QueryOneTimeKeys(ctx context.Context, req *QueryOneTimeKeysRequest, res *QueryOneTimeKeysResponse) error
	PerformMarkAsStaleIfNeeded(ctx context.Context, req *PerformMarkAsStaleRequest, res *struct{}) error
}

type FederationKeyAPI interface {
	UploadDeviceKeysAPI
	QueryKeys(ctx context.Context, req *QueryKeysRequest, res *QueryKeysResponse) error
	QuerySignatures(ctx context.Context, req *QuerySignaturesRequest, res *QuerySignaturesResponse) error
	QueryDeviceMessages(ctx context.Context, req *QueryDeviceMessagesRequest, res *QueryDeviceMessagesResponse) error
	PerformClaimKeys(ctx context.Context, req *PerformClaimKeysRequest, res *PerformClaimKeysResponse) error
}

// KeyError is returned if there was a problem performing/querying the server
type KeyError struct {
	Err                string `json:"error"`
	IsInvalidSignature bool   `json:"is_invalid_signature,omitempty"` // M_INVALID_SIGNATURE
	IsMissingParam     bool   `json:"is_missing_param,omitempty"`     // M_MISSING_PARAM
	IsInvalidParam     bool   `json:"is_invalid_param,omitempty"`     // M_INVALID_PARAM
}

func (k *KeyError) Error() string {
	return k.Err
}

type DeviceMessageType int

const (
	TypeDeviceKeyUpdate DeviceMessageType = iota
	TypeCrossSigningUpdate
)

// DeviceMessage represents the message produced into Kafka by the key server.
type DeviceMessage struct {
	Type                         DeviceMessageType `json:"Type,omitempty"`
	*DeviceKeys                  `json:"DeviceKeys,omitempty"`
	*OutputCrossSigningKeyUpdate `json:"CrossSigningKeyUpdate,omitempty"`
	// A monotonically increasing number which represents device changes for this user.
	StreamID       int64
	DeviceChangeID int64
}

// OutputCrossSigningKeyUpdate is an entry in the signing key update output kafka log
type OutputCrossSigningKeyUpdate struct {
	CrossSigningKeyUpdate `json:"signing_keys"`
}

type CrossSigningKeyUpdate struct {
	MasterKey      *fclient.CrossSigningKey `json:"master_key,omitempty"`
	SelfSigningKey *fclient.CrossSigningKey `json:"self_signing_key,omitempty"`
	UserID         string                   `json:"user_id"`
}

// DeviceKeysEqual returns true if the device keys updates contain the
// same display name and key JSON. This will return false if either of
// the updates is not a device keys update, or if the user ID/device ID
// differ between the two.
func (m1 *DeviceMessage) DeviceKeysEqual(m2 *DeviceMessage) bool {
	if m1.DeviceKeys == nil || m2.DeviceKeys == nil {
		return false
	}
	if m1.UserID != m2.UserID || m1.DeviceID != m2.DeviceID {
		return false
	}
	if m1.DisplayName != m2.DisplayName {
		return false // different display names
	}
	if len(m1.KeyJSON) == 0 || len(m2.KeyJSON) == 0 {
		return false // either is empty
	}
	return bytes.Equal(m1.KeyJSON, m2.KeyJSON)
}

// DeviceKeys represents a set of device keys for a single device
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-keys-upload
type DeviceKeys struct {
	// The user who owns this device
	UserID string
	// The device ID of this device
	DeviceID string
	// The device display name
	DisplayName string
	// The raw device key JSON
	KeyJSON []byte
}

// WithStreamID returns a copy of this device message with the given stream ID
func (k *DeviceKeys) WithStreamID(streamID int64) DeviceMessage {
	return DeviceMessage{
		DeviceKeys: k,
		StreamID:   streamID,
	}
}

// OneTimeKeys represents a set of one-time keys for a single device
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-keys-upload
type OneTimeKeys struct {
	// The user who owns this device
	UserID string
	// The device ID of this device
	DeviceID string
	// A map of algorithm:key_id => key JSON
	KeyJSON map[string]json.RawMessage
}

// Split a key in KeyJSON into algorithm and key ID
func (k *OneTimeKeys) Split(keyIDWithAlgo string) (algo string, keyID string) {
	segments := strings.Split(keyIDWithAlgo, ":")
	return segments[0], segments[1]
}

// OneTimeKeysCount represents the counts of one-time keys for a single device
type OneTimeKeysCount struct {
	// The user who owns this device
	UserID string
	// The device ID of this device
	DeviceID string
	// algorithm to count e.g:
	// {
	//   "curve25519": 10,
	//   "signed_curve25519": 20
	// }
	KeyCount map[string]int
}

// PerformUploadKeysRequest is the request to PerformUploadKeys
type PerformUploadKeysRequest struct {
	UserID      string // Required - User performing the request
	DeviceID    string // Optional - Device performing the request, for fetching OTK count
	DeviceKeys  []DeviceKeys
	OneTimeKeys []OneTimeKeys
	// OnlyDisplayNameUpdates should be `true` if ALL the DeviceKeys are present to update
	// the display name for their respective device, and NOT to modify the keys. The key
	// itself doesn't change but it's easier to pretend upload new keys and reuse the same code paths.
	// Without this flag, requests to modify device display names would delete device keys.
	OnlyDisplayNameUpdates bool
}

// PerformUploadKeysResponse is the response to PerformUploadKeys
type PerformUploadKeysResponse struct {
	// A fatal error when processing e.g database failures
	Error *KeyError
	// A map of user_id -> device_id -> Error for tracking failures.
	KeyErrors        map[string]map[string]*KeyError
	OneTimeKeyCounts []OneTimeKeysCount
}

// PerformDeleteKeysRequest asks the keyserver to forget about certain
// keys, and signatures related to those keys.
type PerformDeleteKeysRequest struct {
	UserID string
	KeyIDs []gomatrixserverlib.KeyID
}

// PerformDeleteKeysResponse is the response to PerformDeleteKeysRequest.
type PerformDeleteKeysResponse struct {
	Error *KeyError
}

// KeyError sets a key error field on KeyErrors
func (r *PerformUploadKeysResponse) KeyError(userID, deviceID string, err *KeyError) {
	if r.KeyErrors[userID] == nil {
		r.KeyErrors[userID] = make(map[string]*KeyError)
	}
	r.KeyErrors[userID][deviceID] = err
}

type PerformClaimKeysRequest struct {
	// Map of user_id to device_id to algorithm name
	OneTimeKeys map[string]map[string]string
	Timeout     time.Duration
}

type PerformClaimKeysResponse struct {
	// Map of user_id to device_id to algorithm:key_id to key JSON
	OneTimeKeys map[string]map[string]map[string]json.RawMessage
	// Map of remote server domain to error JSON
	Failures map[string]interface{}
	// Set if there was a fatal error processing this action
	Error *KeyError
}

type PerformUploadDeviceKeysRequest struct {
	fclient.CrossSigningKeys
	// The user that uploaded the key, should be populated by the clientapi.
	UserID string
}

type PerformUploadDeviceKeysResponse struct {
	Error *KeyError
}

type PerformUploadDeviceSignaturesRequest struct {
	Signatures map[string]map[gomatrixserverlib.KeyID]fclient.CrossSigningForKeyOrDevice
	// The user that uploaded the sig, should be populated by the clientapi.
	UserID string
}

type PerformUploadDeviceSignaturesResponse struct {
	Error *KeyError
}

type QueryKeysRequest struct {
	// The user ID asking for the keys, e.g. if from a client API request.
	// Will not be populated if the key request came from federation.
	UserID string
	// Maps user IDs to a list of devices
	UserToDevices map[string][]string
	Timeout       time.Duration
}

type QueryKeysResponse struct {
	// Map of remote server domain to error JSON
	Failures map[string]interface{}
	// Map of user_id to device_id to device_key
	DeviceKeys map[string]map[string]json.RawMessage
	// Maps of user_id to cross signing key
	MasterKeys      map[string]fclient.CrossSigningKey
	SelfSigningKeys map[string]fclient.CrossSigningKey
	UserSigningKeys map[string]fclient.CrossSigningKey
	// Set if there was a fatal error processing this query
	Error *KeyError
}

type QueryKeyChangesRequest struct {
	// The offset of the last received key event, or sarama.OffsetOldest if this is from the beginning
	Offset int64
	// The inclusive offset where to track key changes up to. Messages with this offset are included in the response.
	// Use types.OffsetNewest if the offset is unknown (then check the response Offset to avoid racing).
	ToOffset int64
}

type QueryKeyChangesResponse struct {
	// The set of users who have had their keys change.
	UserIDs []string
	// The latest offset represented in this response.
	Offset int64
	// Set if there was a problem handling the request.
	Error *KeyError
}

type QueryOneTimeKeysRequest struct {
	// The local user to query OTK counts for
	UserID string
	// The device to query OTK counts for
	DeviceID string
}

type QueryOneTimeKeysResponse struct {
	// OTK key counts, in the extended /sync form described by https://matrix.org/docs/spec/client_server/r0.6.1#id84
	Count OneTimeKeysCount
	Error *KeyError
}

type QueryDeviceMessagesRequest struct {
	UserID string
}

type QueryDeviceMessagesResponse struct {
	// The latest stream ID
	StreamID int64
	Devices  []DeviceMessage
	Error    *KeyError
}

type QuerySignaturesRequest struct {
	// A map of target user ID -> target key/device IDs to retrieve signatures for
	TargetIDs map[string][]gomatrixserverlib.KeyID `json:"target_ids"`
}

type QuerySignaturesResponse struct {
	// A map of target user ID -> target key/device ID -> origin user ID -> origin key/device ID -> signatures
	Signatures map[string]map[gomatrixserverlib.KeyID]types.CrossSigningSigMap
	// A map of target user ID -> cross-signing master key
	MasterKeys map[string]fclient.CrossSigningKey
	// A map of target user ID -> cross-signing self-signing key
	SelfSigningKeys map[string]fclient.CrossSigningKey
	// A map of target user ID -> cross-signing user-signing key
	UserSigningKeys map[string]fclient.CrossSigningKey
	// The request error, if any
	Error *KeyError
}

type PerformMarkAsStaleRequest struct {
	UserID   string
	Domain   gomatrixserverlib.ServerName
	DeviceID string
}
