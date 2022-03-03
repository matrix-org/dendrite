// Copyright 2017 Vector Creations Ltd
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

package shared

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

// Database represents an account database
type Database struct {
	DB                    *sql.DB
	Writer                sqlutil.Writer
	Accounts              tables.AccountsTable
	Profiles              tables.ProfileTable
	AccountDatas          tables.AccountDataTable
	ThreePIDs             tables.ThreePIDTable
	OpenIDTokens          tables.OpenIDTable
	KeyBackups            tables.KeyBackupTable
	KeyBackupVersions     tables.KeyBackupVersionTable
	Devices               tables.DevicesTable
	LoginTokens           tables.LoginTokenTable
	Notifications         tables.NotificationTable
	Pushers               tables.PusherTable
	LoginTokenLifetime    time.Duration
	ServerName            gomatrixserverlib.ServerName
	BcryptCost            int
	OpenIDTokenLifetimeMS int64
}

const (
	// The length of generated device IDs
	deviceIDByteLength   = 6
	loginTokenByteLength = 32
)

// GetAccountByPassword returns the account associated with the given localpart and password.
// Returns sql.ErrNoRows if no account exists which matches the given localpart.
func (d *Database) GetAccountByPassword(
	ctx context.Context, localpart, plaintextPassword string,
) (*api.Account, error) {
	hash, err := d.Accounts.SelectPasswordHash(ctx, localpart)
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintextPassword)); err != nil {
		return nil, err
	}
	return d.Accounts.SelectAccountByLocalpart(ctx, localpart)
}

// GetProfileByLocalpart returns the profile associated with the given localpart.
// Returns sql.ErrNoRows if no profile exists which matches the given localpart.
func (d *Database) GetProfileByLocalpart(
	ctx context.Context, localpart string,
) (*authtypes.Profile, error) {
	return d.Profiles.SelectProfileByLocalpart(ctx, localpart)
}

// SetAvatarURL updates the avatar URL of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetAvatarURL(
	ctx context.Context, localpart string, avatarURL string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.Profiles.SetAvatarURL(ctx, txn, localpart, avatarURL)
	})
}

// SetDisplayName updates the display name of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetDisplayName(
	ctx context.Context, localpart string, displayName string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.Profiles.SetDisplayName(ctx, txn, localpart, displayName)
	})
}

// SetPassword sets the account password to the given hash.
func (d *Database) SetPassword(
	ctx context.Context, localpart, plaintextPassword string,
) error {
	hash, err := d.hashPassword(plaintextPassword)
	if err != nil {
		return err
	}
	return d.Writer.Do(nil, nil, func(txn *sql.Tx) error {
		return d.Accounts.UpdatePassword(ctx, localpart, hash)
	})
}

// CreateAccount makes a new account with the given login name and password, and creates an empty profile
// for this account. If no password is supplied, the account will be a passwordless account. If the
// account already exists, it will return nil, ErrUserExists.
func (d *Database) CreateAccount(
	ctx context.Context, localpart, plaintextPassword, appserviceID string, accountType api.AccountType,
) (acc *api.Account, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		// For guest accounts, we create a new numeric local part
		if accountType == api.AccountTypeGuest {
			var numLocalpart int64
			numLocalpart, err = d.Accounts.SelectNewNumericLocalpart(ctx, txn)
			if err != nil {
				return err
			}
			localpart = strconv.FormatInt(numLocalpart, 10)
			plaintextPassword = ""
			appserviceID = ""
		}
		acc, err = d.createAccount(ctx, txn, localpart, plaintextPassword, appserviceID, accountType)
		return err
	})
	return
}

// WARNING! This function assumes that the relevant mutexes have already
// been taken out by the caller (e.g. CreateAccount or CreateGuestAccount).
func (d *Database) createAccount(
	ctx context.Context, txn *sql.Tx, localpart, plaintextPassword, appserviceID string, accountType api.AccountType,
) (*api.Account, error) {
	var err error
	var account *api.Account
	// Generate a password hash if this is not a password-less user
	hash := ""
	if plaintextPassword != "" {
		hash, err = d.hashPassword(plaintextPassword)
		if err != nil {
			return nil, err
		}
	}
	if account, err = d.Accounts.InsertAccount(ctx, txn, localpart, hash, appserviceID, accountType); err != nil {
		return nil, sqlutil.ErrUserExists
	}
	if err = d.Profiles.InsertProfile(ctx, txn, localpart); err != nil {
		return nil, err
	}
	pushRuleSets := pushrules.DefaultAccountRuleSets(localpart, d.ServerName)
	prbs, err := json.Marshal(pushRuleSets)
	if err != nil {
		return nil, err
	}
	if err = d.AccountDatas.InsertAccountData(ctx, txn, localpart, "", "m.push_rules", json.RawMessage(prbs)); err != nil {
		return nil, err
	}
	return account, nil
}

// SaveAccountData saves new account data for a given user and a given room.
// If the account data is not specific to a room, the room ID should be an empty string
// If an account data already exists for a given set (user, room, data type), it will
// update the corresponding row with the new content
// Returns a SQL error if there was an issue with the insertion/update
func (d *Database) SaveAccountData(
	ctx context.Context, localpart, roomID, dataType string, content json.RawMessage,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.AccountDatas.InsertAccountData(ctx, txn, localpart, roomID, dataType, content)
	})
}

// GetAccountData returns account data related to a given localpart
// If no account data could be found, returns an empty arrays
// Returns an error if there was an issue with the retrieval
func (d *Database) GetAccountData(ctx context.Context, localpart string) (
	global map[string]json.RawMessage,
	rooms map[string]map[string]json.RawMessage,
	err error,
) {
	return d.AccountDatas.SelectAccountData(ctx, localpart)
}

// GetAccountDataByType returns account data matching a given
// localpart, room ID and type.
// If no account data could be found, returns nil
// Returns an error if there was an issue with the retrieval
func (d *Database) GetAccountDataByType(
	ctx context.Context, localpart, roomID, dataType string,
) (data json.RawMessage, err error) {
	return d.AccountDatas.SelectAccountDataByType(
		ctx, localpart, roomID, dataType,
	)
}

// GetNewNumericLocalpart generates and returns a new unused numeric localpart
func (d *Database) GetNewNumericLocalpart(
	ctx context.Context,
) (int64, error) {
	return d.Accounts.SelectNewNumericLocalpart(ctx, nil)
}

func (d *Database) hashPassword(plaintext string) (hash string, err error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), d.BcryptCost)
	return string(hashBytes), err
}

// Err3PIDInUse is the error returned when trying to save an association involving
// a third-party identifier which is already associated to a local user.
var Err3PIDInUse = errors.New("this third-party identifier is already in use")

// SaveThreePIDAssociation saves the association between a third party identifier
// and a local Matrix user (identified by the user's ID's local part).
// If the third-party identifier is already part of an association, returns Err3PIDInUse.
// Returns an error if there was a problem talking to the database.
func (d *Database) SaveThreePIDAssociation(
	ctx context.Context, threepid, localpart, medium string,
) (err error) {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		user, err := d.ThreePIDs.SelectLocalpartForThreePID(
			ctx, txn, threepid, medium,
		)
		if err != nil {
			return err
		}

		if len(user) > 0 {
			return Err3PIDInUse
		}

		return d.ThreePIDs.InsertThreePID(ctx, txn, threepid, medium, localpart)
	})
}

// RemoveThreePIDAssociation removes the association involving a given third-party
// identifier.
// If no association exists involving this third-party identifier, returns nothing.
// If there was a problem talking to the database, returns an error.
func (d *Database) RemoveThreePIDAssociation(
	ctx context.Context, threepid string, medium string,
) (err error) {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.ThreePIDs.DeleteThreePID(ctx, txn, threepid, medium)
	})
}

// GetLocalpartForThreePID looks up the localpart associated with a given third-party
// identifier.
// If no association involves the given third-party idenfitier, returns an empty
// string.
// Returns an error if there was a problem talking to the database.
func (d *Database) GetLocalpartForThreePID(
	ctx context.Context, threepid string, medium string,
) (localpart string, err error) {
	return d.ThreePIDs.SelectLocalpartForThreePID(ctx, nil, threepid, medium)
}

// GetThreePIDsForLocalpart looks up the third-party identifiers associated with
// a given local user.
// If no association is known for this user, returns an empty slice.
// Returns an error if there was an issue talking to the database.
func (d *Database) GetThreePIDsForLocalpart(
	ctx context.Context, localpart string,
) (threepids []authtypes.ThreePID, err error) {
	return d.ThreePIDs.SelectThreePIDsForLocalpart(ctx, localpart)
}

// CheckAccountAvailability checks if the username/localpart is already present
// in the database.
// If the DB returns sql.ErrNoRows the Localpart isn't taken.
func (d *Database) CheckAccountAvailability(ctx context.Context, localpart string) (bool, error) {
	_, err := d.Accounts.SelectAccountByLocalpart(ctx, localpart)
	if err == sql.ErrNoRows {
		return true, nil
	}
	return false, err
}

// GetAccountByLocalpart returns the account associated with the given localpart.
// This function assumes the request is authenticated or the account data is used only internally.
// Returns sql.ErrNoRows if no account exists which matches the given localpart.
func (d *Database) GetAccountByLocalpart(ctx context.Context, localpart string,
) (*api.Account, error) {
	return d.Accounts.SelectAccountByLocalpart(ctx, localpart)
}

// SearchProfiles returns all profiles where the provided localpart or display name
// match any part of the profiles in the database.
func (d *Database) SearchProfiles(ctx context.Context, searchString string, limit int,
) ([]authtypes.Profile, error) {
	return d.Profiles.SelectProfilesBySearch(ctx, searchString, limit)
}

// DeactivateAccount deactivates the user's account, removing all ability for the user to login again.
func (d *Database) DeactivateAccount(ctx context.Context, localpart string) (err error) {
	return d.Writer.Do(nil, nil, func(txn *sql.Tx) error {
		return d.Accounts.DeactivateAccount(ctx, localpart)
	})
}

// CreateOpenIDToken persists a new token that was issued for OpenID Connect
func (d *Database) CreateOpenIDToken(
	ctx context.Context,
	token, localpart string,
) (int64, error) {
	expiresAtMS := time.Now().UnixNano()/int64(time.Millisecond) + d.OpenIDTokenLifetimeMS
	err := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.OpenIDTokens.InsertOpenIDToken(ctx, txn, token, localpart, expiresAtMS)
	})
	return expiresAtMS, err
}

// GetOpenIDTokenAttributes gets the attributes of issued an OIDC auth token
func (d *Database) GetOpenIDTokenAttributes(
	ctx context.Context,
	token string,
) (*api.OpenIDTokenAttributes, error) {
	return d.OpenIDTokens.SelectOpenIDTokenAtrributes(ctx, token)
}

func (d *Database) CreateKeyBackup(
	ctx context.Context, userID, algorithm string, authData json.RawMessage,
) (version string, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		version, err = d.KeyBackupVersions.InsertKeyBackup(ctx, txn, userID, algorithm, authData, "")
		return err
	})
	return
}

func (d *Database) UpdateKeyBackupAuthData(
	ctx context.Context, userID, version string, authData json.RawMessage,
) (err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.KeyBackupVersions.UpdateKeyBackupAuthData(ctx, txn, userID, version, authData)
	})
	return
}

func (d *Database) DeleteKeyBackup(
	ctx context.Context, userID, version string,
) (exists bool, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		exists, err = d.KeyBackupVersions.DeleteKeyBackup(ctx, txn, userID, version)
		return err
	})
	return
}

func (d *Database) GetKeyBackup(
	ctx context.Context, userID, version string,
) (versionResult, algorithm string, authData json.RawMessage, etag string, deleted bool, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		versionResult, algorithm, authData, etag, deleted, err = d.KeyBackupVersions.SelectKeyBackup(ctx, txn, userID, version)
		return err
	})
	return
}

func (d *Database) GetBackupKeys(
	ctx context.Context, version, userID, filterRoomID, filterSessionID string,
) (result map[string]map[string]api.KeyBackupSession, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if filterSessionID != "" {
			result, err = d.KeyBackups.SelectKeysByRoomIDAndSessionID(ctx, txn, userID, version, filterRoomID, filterSessionID)
			return err
		}
		if filterRoomID != "" {
			result, err = d.KeyBackups.SelectKeysByRoomID(ctx, txn, userID, version, filterRoomID)
			return err
		}
		result, err = d.KeyBackups.SelectKeys(ctx, txn, userID, version)
		return err
	})
	return
}

func (d *Database) CountBackupKeys(
	ctx context.Context, version, userID string,
) (count int64, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		count, err = d.KeyBackups.CountKeys(ctx, txn, userID, version)
		if err != nil {
			return err
		}
		return nil
	})
	return
}

// nolint:nakedret
func (d *Database) UpsertBackupKeys(
	ctx context.Context, version, userID string, uploads []api.InternalKeyBackupSession,
) (count int64, etag string, err error) {
	// wrap the following logic in a txn to ensure we atomically upload keys
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		_, _, _, oldETag, deleted, err := d.KeyBackupVersions.SelectKeyBackup(ctx, txn, userID, version)
		if err != nil {
			return err
		}
		if deleted {
			return fmt.Errorf("backup was deleted")
		}
		// pull out all keys for this (user_id, version)
		existingKeys, err := d.KeyBackups.SelectKeys(ctx, txn, userID, version)
		if err != nil {
			return err
		}

		changed := false
		// loop over all the new keys (which should be smaller than the set of backed up keys)
		for _, newKey := range uploads {
			// if we have a matching (room_id, session_id), we may need to update the key if it meets some rules, check them.
			existingRoom := existingKeys[newKey.RoomID]
			if existingRoom != nil {
				existingSession, ok := existingRoom[newKey.SessionID]
				if ok {
					if existingSession.ShouldReplaceRoomKey(&newKey.KeyBackupSession) {
						err = d.KeyBackups.UpdateBackupKey(ctx, txn, userID, version, newKey)
						changed = true
						if err != nil {
							return fmt.Errorf("d.KeyBackups.UpdateBackupKey: %w", err)
						}
					}
					// if we shouldn't replace the key we do nothing with it
					continue
				}
			}
			// if we're here, either the room or session are new, either way, we insert
			err = d.KeyBackups.InsertBackupKey(ctx, txn, userID, version, newKey)
			changed = true
			if err != nil {
				return fmt.Errorf("d.KeyBackups.InsertBackupKey: %w", err)
			}
		}

		count, err = d.KeyBackups.CountKeys(ctx, txn, userID, version)
		if err != nil {
			return err
		}
		if changed {
			// update the etag
			var newETag string
			if oldETag == "" {
				newETag = "1"
			} else {
				oldETagInt, err := strconv.ParseInt(oldETag, 10, 64)
				if err != nil {
					return fmt.Errorf("failed to parse old etag: %s", err)
				}
				newETag = strconv.FormatInt(oldETagInt+1, 10)
			}
			etag = newETag
			return d.KeyBackupVersions.UpdateKeyBackupETag(ctx, txn, userID, version, newETag)
		} else {
			etag = oldETag
		}

		return nil
	})
	return
}

// GetDeviceByAccessToken returns the device matching the given access token.
// Returns sql.ErrNoRows if no matching device was found.
func (d *Database) GetDeviceByAccessToken(
	ctx context.Context, token string,
) (*api.Device, error) {
	return d.Devices.SelectDeviceByToken(ctx, token)
}

// GetDeviceByID returns the device matching the given ID.
// Returns sql.ErrNoRows if no matching device was found.
func (d *Database) GetDeviceByID(
	ctx context.Context, localpart, deviceID string,
) (*api.Device, error) {
	return d.Devices.SelectDeviceByID(ctx, localpart, deviceID)
}

// GetDevicesByLocalpart returns the devices matching the given localpart.
func (d *Database) GetDevicesByLocalpart(
	ctx context.Context, localpart string,
) ([]api.Device, error) {
	return d.Devices.SelectDevicesByLocalpart(ctx, nil, localpart, "")
}

func (d *Database) GetDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error) {
	return d.Devices.SelectDevicesByID(ctx, deviceIDs)
}

// CreateDevice makes a new device associated with the given user ID localpart.
// If there is already a device with the same device ID for this user, that access token will be revoked
// and replaced with the given accessToken. If the given accessToken is already in use for another device,
// an error will be returned.
// If no device ID is given one is generated.
// Returns the device on success.
func (d *Database) CreateDevice(
	ctx context.Context, localpart string, deviceID *string, accessToken string,
	displayName *string, ipAddr, userAgent string,
) (dev *api.Device, returnErr error) {
	if deviceID != nil {
		returnErr = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
			var err error
			// Revoke existing tokens for this device
			if err = d.Devices.DeleteDevice(ctx, txn, *deviceID, localpart); err != nil {
				return err
			}

			dev, err = d.Devices.InsertDevice(ctx, txn, *deviceID, localpart, accessToken, displayName, ipAddr, userAgent)
			return err
		})
	} else {
		// We generate device IDs in a loop in case its already taken.
		// We cap this at going round 5 times to ensure we don't spin forever
		var newDeviceID string
		for i := 1; i <= 5; i++ {
			newDeviceID, returnErr = generateDeviceID()
			if returnErr != nil {
				return
			}

			returnErr = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
				var err error
				dev, err = d.Devices.InsertDevice(ctx, txn, newDeviceID, localpart, accessToken, displayName, ipAddr, userAgent)
				return err
			})
			if returnErr == nil {
				return
			}
		}
	}
	return
}

// generateDeviceID creates a new device id. Returns an error if failed to generate
// random bytes.
func generateDeviceID() (string, error) {
	b := make([]byte, deviceIDByteLength)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	// url-safe no padding
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// UpdateDevice updates the given device with the display name.
// Returns SQL error if there are problems and nil on success.
func (d *Database) UpdateDevice(
	ctx context.Context, localpart, deviceID string, displayName *string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.Devices.UpdateDeviceName(ctx, txn, localpart, deviceID, displayName)
	})
}

// RemoveDevice revokes a device by deleting the entry in the database
// matching with the given device ID and user ID localpart.
// If the device doesn't exist, it will not return an error
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemoveDevice(
	ctx context.Context, deviceID, localpart string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if err := d.Devices.DeleteDevice(ctx, txn, deviceID, localpart); err != sql.ErrNoRows {
			return err
		}
		return nil
	})
}

// RemoveDevices revokes one or more devices by deleting the entry in the database
// matching with the given device IDs and user ID localpart.
// If the devices don't exist, it will not return an error
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemoveDevices(
	ctx context.Context, localpart string, devices []string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if err := d.Devices.DeleteDevices(ctx, txn, localpart, devices); err != sql.ErrNoRows {
			return err
		}
		return nil
	})
}

// RemoveAllDevices revokes devices by deleting the entry in the
// database matching the given user ID localpart.
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemoveAllDevices(
	ctx context.Context, localpart, exceptDeviceID string,
) (devices []api.Device, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		devices, err = d.Devices.SelectDevicesByLocalpart(ctx, txn, localpart, exceptDeviceID)
		if err != nil {
			return err
		}
		if err := d.Devices.DeleteDevicesByLocalpart(ctx, txn, localpart, exceptDeviceID); err != sql.ErrNoRows {
			return err
		}
		return nil
	})
	return
}

// UpdateDeviceLastSeen updates a the last seen timestamp and the ip address
func (d *Database) UpdateDeviceLastSeen(ctx context.Context, localpart, deviceID, ipAddr string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.Devices.UpdateDeviceLastSeen(ctx, txn, localpart, deviceID, ipAddr)
	})
}

// CreateLoginToken generates a token, stores and returns it. The lifetime is
// determined by the loginTokenLifetime given to the Database constructor.
func (d *Database) CreateLoginToken(ctx context.Context, data *api.LoginTokenData) (*api.LoginTokenMetadata, error) {
	tok, err := generateLoginToken()
	if err != nil {
		return nil, err
	}
	meta := &api.LoginTokenMetadata{
		Token:      tok,
		Expiration: time.Now().Add(d.LoginTokenLifetime),
	}

	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.LoginTokens.InsertLoginToken(ctx, txn, meta, data)
	})
	if err != nil {
		return nil, err
	}

	return meta, nil
}

func generateLoginToken() (string, error) {
	b := make([]byte, loginTokenByteLength)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// RemoveLoginToken removes the named token (and may clean up other expired tokens).
func (d *Database) RemoveLoginToken(ctx context.Context, token string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.LoginTokens.DeleteLoginToken(ctx, txn, token)
	})
}

// GetLoginTokenDataByToken returns the data associated with the given token.
// May return sql.ErrNoRows.
func (d *Database) GetLoginTokenDataByToken(ctx context.Context, token string) (*api.LoginTokenData, error) {
	return d.LoginTokens.SelectLoginToken(ctx, token)
}

func (d *Database) InsertNotification(ctx context.Context, localpart, eventID string, pos int64, tweaks map[string]interface{}, n *api.Notification) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.Notifications.Insert(ctx, txn, localpart, eventID, pos, pushrules.BoolTweakOr(tweaks, pushrules.HighlightTweak, false), n)
	})
}

func (d *Database) DeleteNotificationsUpTo(ctx context.Context, localpart, roomID string, pos int64) (affected bool, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		affected, err = d.Notifications.DeleteUpTo(ctx, txn, localpart, roomID, pos)
		return err
	})
	return
}

func (d *Database) SetNotificationsRead(ctx context.Context, localpart, roomID string, pos int64, b bool) (affected bool, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		affected, err = d.Notifications.UpdateRead(ctx, txn, localpart, roomID, pos, b)
		return err
	})
	return
}

func (d *Database) GetNotifications(ctx context.Context, localpart string, fromID int64, limit int, filter tables.NotificationFilter) ([]*api.Notification, int64, error) {
	return d.Notifications.Select(ctx, nil, localpart, fromID, limit, filter)
}

func (d *Database) GetNotificationCount(ctx context.Context, localpart string, filter tables.NotificationFilter) (int64, error) {
	return d.Notifications.SelectCount(ctx, nil, localpart, filter)
}

func (d *Database) GetRoomNotificationCounts(ctx context.Context, localpart, roomID string) (total int64, highlight int64, _ error) {
	return d.Notifications.SelectRoomCounts(ctx, nil, localpart, roomID)
}

func (d *Database) UpsertPusher(
	ctx context.Context, p api.Pusher, localpart string,
) error {
	data, err := json.Marshal(p.Data)
	if err != nil {
		return err
	}
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.Pushers.InsertPusher(
			ctx, txn,
			p.SessionID,
			p.PushKey,
			p.PushKeyTS,
			p.Kind,
			p.AppID,
			p.AppDisplayName,
			p.DeviceDisplayName,
			p.ProfileTag,
			p.Language,
			string(data),
			localpart)
	})
}

// GetPushers returns the pushers matching the given localpart.
func (d *Database) GetPushers(
	ctx context.Context, localpart string,
) ([]api.Pusher, error) {
	return d.Pushers.SelectPushers(ctx, nil, localpart)
}

// RemovePusher deletes one pusher
// Invoked when `append` is true and `kind` is null in
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-pushers-set
func (d *Database) RemovePusher(
	ctx context.Context, appid, pushkey, localpart string,
) error {
	return d.Writer.Do(nil, nil, func(txn *sql.Tx) error {
		err := d.Pushers.DeletePusher(ctx, txn, appid, pushkey, localpart)
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	})
}

// RemovePushers deletes all pushers that match given App Id and Push Key pair.
// Invoked when `append` parameter is false in
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-pushers-set
func (d *Database) RemovePushers(
	ctx context.Context, appid, pushkey string,
) error {
	return d.Writer.Do(nil, nil, func(txn *sql.Tx) error {
		return d.Pushers.DeletePushers(ctx, txn, appid, pushkey)
	})
}
