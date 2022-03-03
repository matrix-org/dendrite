// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package tables

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type AccountDataTable interface {
	InsertAccountData(ctx context.Context, txn *sql.Tx, localpart, roomID, dataType string, content json.RawMessage) error
	SelectAccountData(ctx context.Context, localpart string) (map[string]json.RawMessage, map[string]map[string]json.RawMessage, error)
	SelectAccountDataByType(ctx context.Context, localpart, roomID, dataType string) (data json.RawMessage, err error)
}

type AccountsTable interface {
	InsertAccount(ctx context.Context, txn *sql.Tx, localpart, hash, appserviceID string, accountType api.AccountType) (*api.Account, error)
	UpdatePassword(ctx context.Context, localpart, passwordHash string) (err error)
	DeactivateAccount(ctx context.Context, localpart string) (err error)
	SelectPasswordHash(ctx context.Context, localpart string) (hash string, err error)
	SelectAccountByLocalpart(ctx context.Context, localpart string) (*api.Account, error)
	SelectNewNumericLocalpart(ctx context.Context, txn *sql.Tx) (id int64, err error)
}

type DevicesTable interface {
	InsertDevice(ctx context.Context, txn *sql.Tx, id, localpart, accessToken string, displayName *string, ipAddr, userAgent string) (*api.Device, error)
	DeleteDevice(ctx context.Context, txn *sql.Tx, id, localpart string) error
	DeleteDevices(ctx context.Context, txn *sql.Tx, localpart string, devices []string) error
	DeleteDevicesByLocalpart(ctx context.Context, txn *sql.Tx, localpart, exceptDeviceID string) error
	UpdateDeviceName(ctx context.Context, txn *sql.Tx, localpart, deviceID string, displayName *string) error
	SelectDeviceByToken(ctx context.Context, accessToken string) (*api.Device, error)
	SelectDeviceByID(ctx context.Context, localpart, deviceID string) (*api.Device, error)
	SelectDevicesByLocalpart(ctx context.Context, txn *sql.Tx, localpart, exceptDeviceID string) ([]api.Device, error)
	SelectDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error)
	UpdateDeviceLastSeen(ctx context.Context, txn *sql.Tx, localpart, deviceID, ipAddr string) error
}

type KeyBackupTable interface {
	CountKeys(ctx context.Context, txn *sql.Tx, userID, version string) (count int64, err error)
	InsertBackupKey(ctx context.Context, txn *sql.Tx, userID, version string, key api.InternalKeyBackupSession) (err error)
	UpdateBackupKey(ctx context.Context, txn *sql.Tx, userID, version string, key api.InternalKeyBackupSession) (err error)
	SelectKeys(ctx context.Context, txn *sql.Tx, userID, version string) (map[string]map[string]api.KeyBackupSession, error)
	SelectKeysByRoomID(ctx context.Context, txn *sql.Tx, userID, version, roomID string) (map[string]map[string]api.KeyBackupSession, error)
	SelectKeysByRoomIDAndSessionID(ctx context.Context, txn *sql.Tx, userID, version, roomID, sessionID string) (map[string]map[string]api.KeyBackupSession, error)
}

type KeyBackupVersionTable interface {
	InsertKeyBackup(ctx context.Context, txn *sql.Tx, userID, algorithm string, authData json.RawMessage, etag string) (version string, err error)
	UpdateKeyBackupAuthData(ctx context.Context, txn *sql.Tx, userID, version string, authData json.RawMessage) error
	UpdateKeyBackupETag(ctx context.Context, txn *sql.Tx, userID, version, etag string) error
	DeleteKeyBackup(ctx context.Context, txn *sql.Tx, userID, version string) (bool, error)
	SelectKeyBackup(ctx context.Context, txn *sql.Tx, userID, version string) (versionResult, algorithm string, authData json.RawMessage, etag string, deleted bool, err error)
}

type LoginTokenTable interface {
	InsertLoginToken(ctx context.Context, txn *sql.Tx, metadata *api.LoginTokenMetadata, data *api.LoginTokenData) error
	DeleteLoginToken(ctx context.Context, txn *sql.Tx, token string) error
	SelectLoginToken(ctx context.Context, token string) (*api.LoginTokenData, error)
}

type OpenIDTable interface {
	InsertOpenIDToken(ctx context.Context, txn *sql.Tx, token, localpart string, expiresAtMS int64) (err error)
	SelectOpenIDTokenAtrributes(ctx context.Context, token string) (*api.OpenIDTokenAttributes, error)
}

type ProfileTable interface {
	InsertProfile(ctx context.Context, txn *sql.Tx, localpart string) error
	SelectProfileByLocalpart(ctx context.Context, localpart string) (*authtypes.Profile, error)
	SetAvatarURL(ctx context.Context, txn *sql.Tx, localpart string, avatarURL string) (err error)
	SetDisplayName(ctx context.Context, txn *sql.Tx, localpart string, displayName string) (err error)
	SelectProfilesBySearch(ctx context.Context, searchString string, limit int) ([]authtypes.Profile, error)
}

type ThreePIDTable interface {
	SelectLocalpartForThreePID(ctx context.Context, txn *sql.Tx, threepid string, medium string) (localpart string, err error)
	SelectThreePIDsForLocalpart(ctx context.Context, localpart string) (threepids []authtypes.ThreePID, err error)
	InsertThreePID(ctx context.Context, txn *sql.Tx, threepid, medium, localpart string) (err error)
	DeleteThreePID(ctx context.Context, txn *sql.Tx, threepid string, medium string) (err error)
}

type PusherTable interface {
	InsertPusher(ctx context.Context, txn *sql.Tx, session_id int64, pushkey string, pushkeyTS gomatrixserverlib.Timestamp, kind api.PusherKind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string) error
	SelectPushers(ctx context.Context, txn *sql.Tx, localpart string) ([]api.Pusher, error)
	DeletePusher(ctx context.Context, txn *sql.Tx, appid, pushkey, localpart string) error
	DeletePushers(ctx context.Context, txn *sql.Tx, appid, pushkey string) error
}

type NotificationTable interface {
	Insert(ctx context.Context, txn *sql.Tx, localpart, eventID string, pos int64, highlight bool, n *api.Notification) error
	DeleteUpTo(ctx context.Context, txn *sql.Tx, localpart, roomID string, pos int64) (affected bool, _ error)
	UpdateRead(ctx context.Context, txn *sql.Tx, localpart, roomID string, pos int64, v bool) (affected bool, _ error)
	Select(ctx context.Context, txn *sql.Tx, localpart string, fromID int64, limit int, filter NotificationFilter) ([]*api.Notification, int64, error)
	SelectCount(ctx context.Context, txn *sql.Tx, localpart string, filter NotificationFilter) (int64, error)
	SelectRoomCounts(ctx context.Context, txn *sql.Tx, localpart, roomID string) (total int64, highlight int64, _ error)
}

type NotificationFilter uint32

const (
	// HighlightNotifications returns notifications that had a
	// "highlight" tweak assigned to them from evaluating push rules.
	HighlightNotifications NotificationFilter = 1 << iota

	// NonHighlightNotifications returns notifications that don't
	// match HighlightNotifications.
	NonHighlightNotifications

	// NoNotifications is a filter to exclude all types of
	// notifications. It's useful as a zero value, but isn't likely to
	// be used in a call to Notifications.Select*.
	NoNotifications NotificationFilter = 0

	// AllNotifications is a filter to include all types of
	// notifications in Notifications.Select*. Note that PostgreSQL
	// balks if this doesn't fit in INTEGER, even though we use
	// uint32.
	AllNotifications NotificationFilter = (1 << 31) - 1
)
