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

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts/sqlite3/deltas"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
)

// Database represents an account database
type Database struct {
	db     *sql.DB
	writer sqlutil.Writer

	sqlutil.PartitionOffsetStatements
	accounts              accountsStatements
	profiles              profilesStatements
	accountDatas          accountDataStatements
	threepids             threepidStatements
	openIDTokens          tokenStatements
	keyBackupVersions     keyBackupVersionStatements
	keyBackups            keyBackupStatements
	serverName            gomatrixserverlib.ServerName
	bcryptCost            int
	openIDTokenLifetimeMS int64

	accountsMu     sync.Mutex
	profilesMu     sync.Mutex
	accountDatasMu sync.Mutex
	threepidsMu    sync.Mutex
}

// NewDatabase creates a new accounts and profiles database
func NewDatabase(dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName, bcryptCost int, openIDTokenLifetimeMS int64) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	d := &Database{
		serverName:            serverName,
		db:                    db,
		writer:                sqlutil.NewExclusiveWriter(),
		bcryptCost:            bcryptCost,
		openIDTokenLifetimeMS: openIDTokenLifetimeMS,
	}

	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	if err = d.accounts.execSchema(db); err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrations()
	deltas.LoadIsActive(m)
	if err = m.RunDeltas(db, dbProperties); err != nil {
		return nil, err
	}

	partitions := sqlutil.PartitionOffsetStatements{}
	if err = partitions.Prepare(db, d.writer, "account"); err != nil {
		return nil, err
	}
	if err = d.accounts.prepare(db, serverName); err != nil {
		return nil, err
	}
	if err = d.profiles.prepare(db); err != nil {
		return nil, err
	}
	if err = d.accountDatas.prepare(db); err != nil {
		return nil, err
	}
	if err = d.threepids.prepare(db); err != nil {
		return nil, err
	}
	if err = d.openIDTokens.prepare(db, serverName); err != nil {
		return nil, err
	}
	if err = d.keyBackupVersions.prepare(db); err != nil {
		return nil, err
	}
	if err = d.keyBackups.prepare(db); err != nil {
		return nil, err
	}

	return d, nil
}

// GetAccountByPassword returns the account associated with the given localpart and password.
// Returns sql.ErrNoRows if no account exists which matches the given localpart.
func (d *Database) GetAccountByPassword(
	ctx context.Context, localpart, plaintextPassword string,
) (*api.Account, error) {
	hash, err := d.accounts.selectPasswordHash(ctx, localpart)
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintextPassword)); err != nil {
		return nil, err
	}
	return d.accounts.selectAccountByLocalpart(ctx, localpart)
}

// GetProfileByLocalpart returns the profile associated with the given localpart.
// Returns sql.ErrNoRows if no profile exists which matches the given localpart.
func (d *Database) GetProfileByLocalpart(
	ctx context.Context, localpart string,
) (*authtypes.Profile, error) {
	return d.profiles.selectProfileByLocalpart(ctx, localpart)
}

// SetAvatarURL updates the avatar URL of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetAvatarURL(
	ctx context.Context, localpart string, avatarURL string,
) error {
	d.profilesMu.Lock()
	defer d.profilesMu.Unlock()
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.profiles.setAvatarURL(ctx, txn, localpart, avatarURL)
	})
}

// SetDisplayName updates the display name of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetDisplayName(
	ctx context.Context, localpart string, displayName string,
) error {
	d.profilesMu.Lock()
	defer d.profilesMu.Unlock()
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.profiles.setDisplayName(ctx, txn, localpart, displayName)
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
	return d.writer.Do(nil, nil, func(txn *sql.Tx) error {
		return d.accounts.updatePassword(ctx, localpart, hash)
	})
}

// CreateGuestAccount makes a new guest account and creates an empty profile
// for this account.
func (d *Database) CreateGuestAccount(ctx context.Context) (acc *api.Account, err error) {
	// We need to lock so we sequentially create numeric localparts. If we don't, two calls to
	// this function will cause the same number to be selected and one will fail with 'database is locked'
	// when the first txn upgrades to a write txn. We also need to lock the account creation else we can
	// race with CreateAccount
	// We know we'll be the only process since this is sqlite ;) so a lock here will be all that is needed.
	d.profilesMu.Lock()
	d.accountDatasMu.Lock()
	d.accountsMu.Lock()
	defer d.profilesMu.Unlock()
	defer d.accountDatasMu.Unlock()
	defer d.accountsMu.Unlock()
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		var numLocalpart int64
		numLocalpart, err = d.accounts.selectNewNumericLocalpart(ctx, txn)
		if err != nil {
			return err
		}
		localpart := strconv.FormatInt(numLocalpart, 10)
		acc, err = d.createAccount(ctx, txn, localpart, "", "")
		return err
	})
	return acc, err
}

// CreateAccount makes a new account with the given login name and password, and creates an empty profile
// for this account. If no password is supplied, the account will be a passwordless account. If the
// account already exists, it will return nil, ErrUserExists.
func (d *Database) CreateAccount(
	ctx context.Context, localpart, plaintextPassword, appserviceID string,
) (acc *api.Account, err error) {
	// Create one account at a time else we can get 'database is locked'.
	d.profilesMu.Lock()
	d.accountDatasMu.Lock()
	d.accountsMu.Lock()
	defer d.profilesMu.Unlock()
	defer d.accountDatasMu.Unlock()
	defer d.accountsMu.Unlock()
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		acc, err = d.createAccount(ctx, txn, localpart, plaintextPassword, appserviceID)
		return err
	})
	return
}

// WARNING! This function assumes that the relevant mutexes have already
// been taken out by the caller (e.g. CreateAccount or CreateGuestAccount).
func (d *Database) createAccount(
	ctx context.Context, txn *sql.Tx, localpart, plaintextPassword, appserviceID string,
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
	if account, err = d.accounts.insertAccount(ctx, txn, localpart, hash, appserviceID); err != nil {
		return nil, sqlutil.ErrUserExists
	}
	if err = d.profiles.insertProfile(ctx, txn, localpart); err != nil {
		return nil, err
	}
	if err = d.accountDatas.insertAccountData(ctx, txn, localpart, "", "m.push_rules", json.RawMessage(`{
		"global": {
			"content": [],
			"override": [],
			"room": [],
			"sender": [],
			"underride": []
		}
	}`)); err != nil {
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
	d.accountDatasMu.Lock()
	defer d.accountDatasMu.Unlock()
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.accountDatas.insertAccountData(ctx, txn, localpart, roomID, dataType, content)
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
	return d.accountDatas.selectAccountData(ctx, localpart)
}

// GetAccountDataByType returns account data matching a given
// localpart, room ID and type.
// If no account data could be found, returns nil
// Returns an error if there was an issue with the retrieval
func (d *Database) GetAccountDataByType(
	ctx context.Context, localpart, roomID, dataType string,
) (data json.RawMessage, err error) {
	return d.accountDatas.selectAccountDataByType(
		ctx, localpart, roomID, dataType,
	)
}

// GetNewNumericLocalpart generates and returns a new unused numeric localpart
func (d *Database) GetNewNumericLocalpart(
	ctx context.Context,
) (int64, error) {
	return d.accounts.selectNewNumericLocalpart(ctx, nil)
}

func (d *Database) hashPassword(plaintext string) (hash string, err error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), d.bcryptCost)
	return string(hashBytes), err
}

// Err3PIDInUse is the error returned when trying to save an association involving
// a third-party identifier which is already associated to a local user.
var Err3PIDInUse = errors.New("This third-party identifier is already in use")

// SaveThreePIDAssociation saves the association between a third party identifier
// and a local Matrix user (identified by the user's ID's local part).
// If the third-party identifier is already part of an association, returns Err3PIDInUse.
// Returns an error if there was a problem talking to the database.
func (d *Database) SaveThreePIDAssociation(
	ctx context.Context, threepid, localpart, medium string,
) (err error) {
	d.threepidsMu.Lock()
	defer d.threepidsMu.Unlock()
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		user, err := d.threepids.selectLocalpartForThreePID(
			ctx, txn, threepid, medium,
		)
		if err != nil {
			return err
		}

		if len(user) > 0 {
			return Err3PIDInUse
		}

		return d.threepids.insertThreePID(ctx, txn, threepid, medium, localpart)
	})
}

// RemoveThreePIDAssociation removes the association involving a given third-party
// identifier.
// If no association exists involving this third-party identifier, returns nothing.
// If there was a problem talking to the database, returns an error.
func (d *Database) RemoveThreePIDAssociation(
	ctx context.Context, threepid string, medium string,
) (err error) {
	d.threepidsMu.Lock()
	defer d.threepidsMu.Unlock()
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.threepids.deleteThreePID(ctx, txn, threepid, medium)
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
	return d.threepids.selectLocalpartForThreePID(ctx, nil, threepid, medium)
}

// GetThreePIDsForLocalpart looks up the third-party identifiers associated with
// a given local user.
// If no association is known for this user, returns an empty slice.
// Returns an error if there was an issue talking to the database.
func (d *Database) GetThreePIDsForLocalpart(
	ctx context.Context, localpart string,
) (threepids []authtypes.ThreePID, err error) {
	return d.threepids.selectThreePIDsForLocalpart(ctx, localpart)
}

// CheckAccountAvailability checks if the username/localpart is already present
// in the database.
// If the DB returns sql.ErrNoRows the Localpart isn't taken.
func (d *Database) CheckAccountAvailability(ctx context.Context, localpart string) (bool, error) {
	_, err := d.accounts.selectAccountByLocalpart(ctx, localpart)
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
	return d.accounts.selectAccountByLocalpart(ctx, localpart)
}

// SearchProfiles returns all profiles where the provided localpart or display name
// match any part of the profiles in the database.
func (d *Database) SearchProfiles(ctx context.Context, searchString string, limit int,
) ([]authtypes.Profile, error) {
	return d.profiles.selectProfilesBySearch(ctx, searchString, limit)
}

// DeactivateAccount deactivates the user's account, removing all ability for the user to login again.
func (d *Database) DeactivateAccount(ctx context.Context, localpart string) (err error) {
	return d.writer.Do(nil, nil, func(txn *sql.Tx) error {
		return d.accounts.deactivateAccount(ctx, localpart)
	})
}

// CreateOpenIDToken persists a new token that was issued for OpenID Connect
func (d *Database) CreateOpenIDToken(
	ctx context.Context,
	token, localpart string,
) (int64, error) {
	expiresAtMS := time.Now().UnixNano()/int64(time.Millisecond) + d.openIDTokenLifetimeMS
	err := d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.openIDTokens.insertToken(ctx, txn, token, localpart, expiresAtMS)
	})
	return expiresAtMS, err
}

// GetOpenIDTokenAttributes gets the attributes of issued an OIDC auth token
func (d *Database) GetOpenIDTokenAttributes(
	ctx context.Context,
	token string,
) (*api.OpenIDTokenAttributes, error) {
	return d.openIDTokens.selectOpenIDTokenAtrributes(ctx, token)
}

func (d *Database) CreateKeyBackup(
	ctx context.Context, userID, algorithm string, authData json.RawMessage,
) (version string, err error) {
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		version, err = d.keyBackupVersions.insertKeyBackup(ctx, txn, userID, algorithm, authData, "")
		return err
	})
	return
}

func (d *Database) UpdateKeyBackupAuthData(
	ctx context.Context, userID, version string, authData json.RawMessage,
) (err error) {
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.keyBackupVersions.updateKeyBackupAuthData(ctx, txn, userID, version, authData)
	})
	return
}

func (d *Database) DeleteKeyBackup(
	ctx context.Context, userID, version string,
) (exists bool, err error) {
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		exists, err = d.keyBackupVersions.deleteKeyBackup(ctx, txn, userID, version)
		return err
	})
	return
}

func (d *Database) GetKeyBackup(
	ctx context.Context, userID, version string,
) (versionResult, algorithm string, authData json.RawMessage, etag string, deleted bool, err error) {
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		versionResult, algorithm, authData, etag, deleted, err = d.keyBackupVersions.selectKeyBackup(ctx, txn, userID, version)
		return err
	})
	return
}

func (d *Database) GetBackupKeys(
	ctx context.Context, version, userID, filterRoomID, filterSessionID string,
) (result map[string]map[string]api.KeyBackupSession, err error) {
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		if filterSessionID != "" {
			result, err = d.keyBackups.selectKeysByRoomIDAndSessionID(ctx, txn, userID, version, filterRoomID, filterSessionID)
			return err
		}
		if filterRoomID != "" {
			result, err = d.keyBackups.selectKeysByRoomID(ctx, txn, userID, version, filterRoomID)
			return err
		}
		result, err = d.keyBackups.selectKeys(ctx, txn, userID, version)
		return err
	})
	return
}

func (d *Database) CountBackupKeys(
	ctx context.Context, version, userID string,
) (count int64, err error) {
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		count, err = d.keyBackups.countKeys(ctx, txn, userID, version)
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
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		_, _, _, oldETag, deleted, err := d.keyBackupVersions.selectKeyBackup(ctx, txn, userID, version)
		if err != nil {
			return err
		}
		if deleted {
			return fmt.Errorf("backup was deleted")
		}
		// pull out all keys for this (user_id, version)
		existingKeys, err := d.keyBackups.selectKeys(ctx, txn, userID, version)
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
						err = d.keyBackups.updateBackupKey(ctx, txn, userID, version, newKey)
						changed = true
						if err != nil {
							return fmt.Errorf("d.keyBackups.updateBackupKey: %w", err)
						}
					}
					// if we shouldn't replace the key we do nothing with it
					continue
				}
			}
			// if we're here, either the room or session are new, either way, we insert
			err = d.keyBackups.insertBackupKey(ctx, txn, userID, version, newKey)
			changed = true
			if err != nil {
				return fmt.Errorf("d.keyBackups.insertBackupKey: %w", err)
			}
		}

		count, err = d.keyBackups.countKeys(ctx, txn, userID, version)
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
			return d.keyBackupVersions.updateKeyBackupETag(ctx, txn, userID, version, newETag)
		} else {
			etag = oldETag
		}

		return nil
	})
	return
}
