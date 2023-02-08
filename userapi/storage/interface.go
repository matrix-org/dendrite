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
	// determined by the loginTokenLifetime given to the Database constructor.
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

type Database interface {
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

type Statistics interface {
	UserStatistics(ctx context.Context) (*types.UserStatistics, *types.DatabaseEngine, error)
	DailyRoomsMessages(ctx context.Context, serverName gomatrixserverlib.ServerName) (stats types.MessageStats, activeRooms, activeE2EERooms int64, err error)
	UpsertDailyRoomsMessages(ctx context.Context, serverName gomatrixserverlib.ServerName, stats types.MessageStats, activeRooms, activeE2EERooms int64) error
}

// Err3PIDInUse is the error returned when trying to save an association involving
// a third-party identifier which is already associated to a local user.
var Err3PIDInUse = errors.New("this third-party identifier is already in use")
