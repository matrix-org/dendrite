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

package storage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/dendrite/userapi/types"
)

type Profile interface {
	GetProfileByLocalpart(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) (*authtypes.Profile, error)
	SearchProfiles(ctx context.Context, searchString string, limit int) ([]authtypes.Profile, error)
	SetAvatarURL(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, avatarURL string) (*authtypes.Profile, bool, error)
	SetDisplayName(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, displayName string) (*authtypes.Profile, bool, error)
}

type Account interface {
	// CreateAccount makes a new account with the given login name and password, and creates an empty profile
	// for this account. If no password is supplied, the account will be a passwordless account. If the
	// account already exists, it will return nil, ErrUserExists.
	CreateAccount(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, plaintextPassword string, appserviceID string, accountType api.AccountType) (*api.Account, error)
	GetAccountByPassword(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, plaintextPassword string) (*api.Account, error)
	GetNewNumericLocalpart(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error)
	CheckAccountAvailability(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) (bool, error)
	GetAccountByLocalpart(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) (*api.Account, error)
	DeactivateAccount(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) (err error)
	SetPassword(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, plaintextPassword string) error
}

type AccountData interface {
	SaveAccountData(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, roomID, dataType string, content json.RawMessage) error
	GetAccountData(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) (global map[string]json.RawMessage, rooms map[string]map[string]json.RawMessage, err error)
	// GetAccountDataByType returns account data matching a given
	// localpart, room ID and type.
	// If no account data could be found, returns nil
	// Returns an error if there was an issue with the retrieval
	GetAccountDataByType(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, roomID, dataType string) (data json.RawMessage, err error)
	QueryPushRules(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) (*pushrules.AccountRuleSets, error)
}

type Device interface {
	GetDeviceByAccessToken(ctx context.Context, token string) (*api.Device, error)
	GetDeviceByID(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, deviceID string) (*api.Device, error)
	GetDevicesByLocalpart(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) ([]api.Device, error)
	GetDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error)
	// CreateDevice makes a new device associated with the given user ID localpart.
	// If there is already a device with the same device ID for this user, that access token will be revoked
	// and replaced with the given accessToken. If the given accessToken is already in use for another device,
	// an error will be returned.
	// If no device ID is given one is generated.
	// Returns the device on success.
	CreateDevice(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, deviceID *string, accessToken string, displayName *string, ipAddr, userAgent string) (dev *api.Device, returnErr error)
	UpdateDevice(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, deviceID string, displayName *string) error
	UpdateDeviceLastSeen(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, deviceID, ipAddr, userAgent string) error
	RemoveDevices(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, devices []string) error
	// RemoveAllDevices deleted all devices for this user. Returns the devices deleted.
	RemoveAllDevices(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, exceptDeviceID string) (devices []api.Device, err error)
}

type KeyBackup interface {
	CreateKeyBackup(ctx context.Context, userID, algorithm string, authData json.RawMessage) (version string, err error)
	UpdateKeyBackupAuthData(ctx context.Context, userID, version string, authData json.RawMessage) (err error)
	DeleteKeyBackup(ctx context.Context, userID, version string) (exists bool, err error)
	GetKeyBackup(ctx context.Context, userID, version string) (versionResult, algorithm string, authData json.RawMessage, etag string, deleted bool, err error)
	UpsertBackupKeys(ctx context.Context, version, userID string, uploads []api.InternalKeyBackupSession) (count int64, etag string, err error)
	GetBackupKeys(ctx context.Context, version, userID, filterRoomID, filterSessionID string) (result map[string]map[string]api.KeyBackupSession, err error)
	CountBackupKeys(ctx context.Context, version, userID string) (count int64, err error)
}

type LoginToken interface {
	// CreateLoginToken generates a token, stores and returns it. The lifetime is
	// determined by the loginTokenLifetime given to the UserDatabase constructor.
	CreateLoginToken(ctx context.Context, data *api.LoginTokenData) (*api.LoginTokenMetadata, error)

	// RemoveLoginToken removes the named token (and may clean up other expired tokens).
	RemoveLoginToken(ctx context.Context, token string) error

	// GetLoginTokenDataByToken returns the data associated with the given token.
	// May return sql.ErrNoRows.
	GetLoginTokenDataByToken(ctx context.Context, token string) (*api.LoginTokenData, error)
}

type OpenID interface {
	CreateOpenIDToken(ctx context.Context, token, userID string) (exp int64, err error)
	GetOpenIDTokenAttributes(ctx context.Context, token string) (*api.OpenIDTokenAttributes, error)
}

type Pusher interface {
	UpsertPusher(ctx context.Context, p api.Pusher, localpart string, serverName gomatrixserverlib.ServerName) error
	GetPushers(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) ([]api.Pusher, error)
	RemovePusher(ctx context.Context, appid, pushkey, localpart string, serverName gomatrixserverlib.ServerName) error
	RemovePushers(ctx context.Context, appid, pushkey string) error
}

type ThreePID interface {
	SaveThreePIDAssociation(ctx context.Context, threepid, localpart string, serverName gomatrixserverlib.ServerName, medium string) (err error)
	RemoveThreePIDAssociation(ctx context.Context, threepid string, medium string) (err error)
	GetLocalpartForThreePID(ctx context.Context, threepid string, medium string) (localpart string, serverName gomatrixserverlib.ServerName, err error)
	GetThreePIDsForLocalpart(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName) (threepids []authtypes.ThreePID, err error)
}

type Notification interface {
	InsertNotification(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, eventID string, pos uint64, tweaks map[string]interface{}, n *api.Notification) error
	DeleteNotificationsUpTo(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, roomID string, pos uint64) (affected bool, err error)
	SetNotificationsRead(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, roomID string, pos uint64, read bool) (affected bool, err error)
	GetNotifications(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, fromID int64, limit int, filter tables.NotificationFilter) ([]*api.Notification, int64, error)
	GetNotificationCount(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, filter tables.NotificationFilter) (int64, error)
	GetRoomNotificationCounts(ctx context.Context, localpart string, serverName gomatrixserverlib.ServerName, roomID string) (total int64, highlight int64, _ error)
	DeleteOldNotifications(ctx context.Context) error
}

type UserDatabase interface {
	Account
	AccountData
	Device
	KeyBackup
	LoginToken
	Notification
	OpenID
	Profile
	Pusher
	Statistics
	ThreePID
}

type KeyChangeDatabase interface {
	// StoreKeyChange stores key change metadata and returns the device change ID which represents the position in the /sync stream for this device change.
	// `userID` is the the user who has changed their keys in some way.
	StoreKeyChange(ctx context.Context, userID string) (int64, error)
}

type KeyDatabase interface {
	KeyChangeDatabase
	// ExistingOneTimeKeys returns a map of keyIDWithAlgorithm to key JSON for the given parameters. If no keys exist with this combination
	// of user/device/key/algorithm 4-uple then it is omitted from the map. Returns an error when failing to communicate with the database.
	ExistingOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error)

	// StoreOneTimeKeys persists the given one-time keys.
	StoreOneTimeKeys(ctx context.Context, keys api.OneTimeKeys) (*api.OneTimeKeysCount, error)

	// OneTimeKeysCount returns a count of all OTKs for this device.
	OneTimeKeysCount(ctx context.Context, userID, deviceID string) (*api.OneTimeKeysCount, error)

	// DeviceKeysJSON populates the KeyJSON for the given keys. If any proided `keys` have a `KeyJSON` or `StreamID` already then it will be replaced.
	DeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error

	// StoreLocalDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced. An empty KeyJSON removes the key
	// for this (user, device).
	// The `StreamID` for each message is set on successful insertion. In the event the key already exists, the existing StreamID is set.
	// Returns an error if there was a problem storing the keys.
	StoreLocalDeviceKeys(ctx context.Context, keys []api.DeviceMessage) error

	// StoreRemoteDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced. An empty KeyJSON removes the key
	// for this (user, device). Does not modify the stream ID for keys. User IDs in `clearUserIDs` will have all their device keys deleted prior
	// to insertion - use this when you have a complete snapshot of a user's keys in order to track device deletions correctly.
	StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage, clearUserIDs []string) error

	// PrevIDsExists returns true if all prev IDs exist for this user.
	PrevIDsExists(ctx context.Context, userID string, prevIDs []int64) (bool, error)

	// DeviceKeysForUser returns the device keys for the device IDs given. If the length of deviceIDs is 0, all devices are selected.
	// If there are some missing keys, they are omitted from the returned slice. There is no ordering on the returned slice.
	DeviceKeysForUser(ctx context.Context, userID string, deviceIDs []string, includeEmpty bool) ([]api.DeviceMessage, error)

	// DeleteDeviceKeys removes the device keys for a given user/device, and any accompanying
	// cross-signing signatures relating to that device.
	DeleteDeviceKeys(ctx context.Context, userID string, deviceIDs []gomatrixserverlib.KeyID) error

	// ClaimKeys based on the 3-uple of user_id, device_id and algorithm name. Returns the keys claimed. Returns no error if a key
	// cannot be claimed or if none exist for this (user, device, algorithm), instead it is omitted from the returned slice.
	ClaimKeys(ctx context.Context, userToDeviceToAlgorithm map[string]map[string]string) ([]api.OneTimeKeys, error)

	// KeyChanges returns a list of user IDs who have modified their keys from the offset given (exclusive) to the offset given (inclusive).
	// A to offset of types.OffsetNewest means no upper limit.
	// Returns the offset of the latest key change.
	KeyChanges(ctx context.Context, fromOffset, toOffset int64) (userIDs []string, latestOffset int64, err error)

	// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
	// If no domains are given, all user IDs with stale device lists are returned.
	StaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error)

	// MarkDeviceListStale sets the stale bit for this user to isStale.
	MarkDeviceListStale(ctx context.Context, userID string, isStale bool) error

	CrossSigningKeysForUser(ctx context.Context, userID string) (map[gomatrixserverlib.CrossSigningKeyPurpose]gomatrixserverlib.CrossSigningKey, error)
	CrossSigningKeysDataForUser(ctx context.Context, userID string) (types.CrossSigningKeyMap, error)
	CrossSigningSigsForTarget(ctx context.Context, originUserID, targetUserID string, targetKeyID gomatrixserverlib.KeyID) (types.CrossSigningSigMap, error)

	StoreCrossSigningKeysForUser(ctx context.Context, userID string, keyMap types.CrossSigningKeyMap) error
	StoreCrossSigningSigsForTarget(ctx context.Context, originUserID string, originKeyID gomatrixserverlib.KeyID, targetUserID string, targetKeyID gomatrixserverlib.KeyID, signature gomatrixserverlib.Base64Bytes) error

	DeleteStaleDeviceLists(
		ctx context.Context,
		userIDs []string,
	) error
}

type Statistics interface {
	UserStatistics(ctx context.Context) (*types.UserStatistics, *types.DatabaseEngine, error)
	DailyRoomsMessages(ctx context.Context, serverName gomatrixserverlib.ServerName) (stats types.MessageStats, activeRooms, activeE2EERooms int64, err error)
	UpsertDailyRoomsMessages(ctx context.Context, serverName gomatrixserverlib.ServerName, stats types.MessageStats, activeRooms, activeE2EERooms int64) error
}

// Err3PIDInUse is the error returned when trying to save an association involving
// a third-party identifier which is already associated to a local user.
var Err3PIDInUse = errors.New("this third-party identifier is already in use")
