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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

type Database interface {
	GetAccountByPassword(ctx context.Context, localpart, plaintextPassword string) (*api.Account, error)
	GetProfileByLocalpart(ctx context.Context, localpart string) (*authtypes.Profile, error)
	SetPassword(ctx context.Context, localpart string, plaintextPassword string) error
	SetAvatarURL(ctx context.Context, localpart string, avatarURL string) error
	SetDisplayName(ctx context.Context, localpart string, displayName string) error
	// CreateAccount makes a new account with the given login name and password, and creates an empty profile
	// for this account. If no password is supplied, the account will be a passwordless account. If the
	// account already exists, it will return nil, ErrUserExists.
	CreateAccount(ctx context.Context, localpart string, plaintextPassword string, appserviceID string, accountType api.AccountType) (*api.Account, error)
	SaveAccountData(ctx context.Context, localpart, roomID, dataType string, content json.RawMessage) error
	GetAccountData(ctx context.Context, localpart string) (global map[string]json.RawMessage, rooms map[string]map[string]json.RawMessage, err error)
	// GetAccountDataByType returns account data matching a given
	// localpart, room ID and type.
	// If no account data could be found, returns nil
	// Returns an error if there was an issue with the retrieval
	GetAccountDataByType(ctx context.Context, localpart, roomID, dataType string) (data json.RawMessage, err error)
	GetNewNumericLocalpart(ctx context.Context) (int64, error)
	SaveThreePIDAssociation(ctx context.Context, threepid, localpart, medium string) (err error)
	RemoveThreePIDAssociation(ctx context.Context, threepid string, medium string) (err error)
	GetLocalpartForThreePID(ctx context.Context, threepid string, medium string) (localpart string, err error)
	GetThreePIDsForLocalpart(ctx context.Context, localpart string) (threepids []authtypes.ThreePID, err error)
	CheckAccountAvailability(ctx context.Context, localpart string) (bool, error)
	GetAccountByLocalpart(ctx context.Context, localpart string) (*api.Account, error)
	SearchProfiles(ctx context.Context, searchString string, limit int) ([]authtypes.Profile, error)
	DeactivateAccount(ctx context.Context, localpart string) (err error)
	CreateOpenIDToken(ctx context.Context, token, localpart string) (exp int64, err error)
	GetOpenIDTokenAttributes(ctx context.Context, token string) (*api.OpenIDTokenAttributes, error)

	// Key backups
	CreateKeyBackup(ctx context.Context, userID, algorithm string, authData json.RawMessage) (version string, err error)
	UpdateKeyBackupAuthData(ctx context.Context, userID, version string, authData json.RawMessage) (err error)
	DeleteKeyBackup(ctx context.Context, userID, version string) (exists bool, err error)
	GetKeyBackup(ctx context.Context, userID, version string) (versionResult, algorithm string, authData json.RawMessage, etag string, deleted bool, err error)
	UpsertBackupKeys(ctx context.Context, version, userID string, uploads []api.InternalKeyBackupSession) (count int64, etag string, err error)
	GetBackupKeys(ctx context.Context, version, userID, filterRoomID, filterSessionID string) (result map[string]map[string]api.KeyBackupSession, err error)
	CountBackupKeys(ctx context.Context, version, userID string) (count int64, err error)

	GetDeviceByAccessToken(ctx context.Context, token string) (*api.Device, error)
	GetDeviceByID(ctx context.Context, localpart, deviceID string) (*api.Device, error)
	GetDevicesByLocalpart(ctx context.Context, localpart string) ([]api.Device, error)
	GetDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error)
	// CreateDevice makes a new device associated with the given user ID localpart.
	// If there is already a device with the same device ID for this user, that access token will be revoked
	// and replaced with the given accessToken. If the given accessToken is already in use for another device,
	// an error will be returned.
	// If no device ID is given one is generated.
	// Returns the device on success.
	CreateDevice(ctx context.Context, localpart string, deviceID *string, accessToken string, displayName *string, ipAddr, userAgent string) (dev *api.Device, returnErr error)
	UpdateDevice(ctx context.Context, localpart, deviceID string, displayName *string) error
	UpdateDeviceLastSeen(ctx context.Context, localpart, deviceID, ipAddr string) error
	RemoveDevice(ctx context.Context, deviceID, localpart string) error
	RemoveDevices(ctx context.Context, localpart string, devices []string) error
	// RemoveAllDevices deleted all devices for this user. Returns the devices deleted.
	RemoveAllDevices(ctx context.Context, localpart, exceptDeviceID string) (devices []api.Device, err error)

	// CreateLoginToken generates a token, stores and returns it. The lifetime is
	// determined by the loginTokenLifetime given to the Database constructor.
	CreateLoginToken(ctx context.Context, data *api.LoginTokenData) (*api.LoginTokenMetadata, error)

	// RemoveLoginToken removes the named token (and may clean up other expired tokens).
	RemoveLoginToken(ctx context.Context, token string) error

	// GetLoginTokenDataByToken returns the data associated with the given token.
	// May return sql.ErrNoRows.
	GetLoginTokenDataByToken(ctx context.Context, token string) (*api.LoginTokenData, error)

	InsertNotification(ctx context.Context, localpart, eventID string, tweaks map[string]interface{}, n *api.Notification) error
	DeleteNotificationsUpTo(ctx context.Context, localpart, roomID, upToEventID string) (affected bool, err error)
	SetNotificationsRead(ctx context.Context, localpart, roomID, upToEventID string, b bool) (affected bool, err error)
	GetNotifications(ctx context.Context, localpart string, fromID int64, limit int, filter tables.NotificationFilter) ([]*api.Notification, int64, error)
	GetNotificationCount(ctx context.Context, localpart string, filter tables.NotificationFilter) (int64, error)
	GetRoomNotificationCounts(ctx context.Context, localpart, roomID string) (total int64, highlight int64, _ error)

	UpsertPusher(ctx context.Context, p api.Pusher, localpart string) error
	GetPushers(ctx context.Context, localpart string) ([]api.Pusher, error)
	RemovePusher(ctx context.Context, appid, pushkey, localpart string) error
	RemovePushers(ctx context.Context, appid, pushkey string) error
}

// Err3PIDInUse is the error returned when trying to save an association involving
// a third-party identifier which is already associated to a local user.
var Err3PIDInUse = errors.New("this third-party identifier is already in use")
