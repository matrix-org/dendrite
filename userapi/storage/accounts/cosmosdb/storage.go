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

package cosmosdb

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	// "sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
)

// Database represents an account database
type Database struct {
	sqlutil.PartitionOffsetStatements
	accounts              accountsStatements
	profiles              profilesStatements
	accountDatas          accountDataStatements
	threepids             threepidStatements
	openIDTokens          tokenStatements
	serverName            gomatrixserverlib.ServerName
	bcryptCost            int
	openIDTokenLifetimeMS int64

	databaseName string
	connection   cosmosdbapi.CosmosConnection
}

// NewDatabase creates a new accounts and profiles database
func NewDatabase(dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName, bcryptCost int, openIDTokenLifetimeMS int64) (*Database, error) {
	connString := cosmosdbutil.GetConnectionString(&dbProperties.ConnectionString)
	connMap := cosmosdbutil.GetConnectionProperties(string(connString))
	accountEndpoint := connMap["AccountEndpoint"]
	accountKey := connMap["AccountKey"]
	conn := cosmosdbapi.GetCosmosConnection(accountEndpoint, accountKey)

	d := &Database{
		serverName:   serverName,
		databaseName: "userapi",
		connection:   conn,
		// db:                    db,
		// writer:                sqlutil.NewExclusiveWriter(),
		// bcryptCost:            bcryptCost,
		// openIDTokenLifetimeMS: openIDTokenLifetimeMS,
	}

	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	// if err = d.accounts.execSchema(db); err != nil {
	// 	return nil, err
	// }
	// m := sqlutil.NewMigrations()
	// deltas.LoadIsActive(m)
	// if err = m.RunDeltas(db, dbProperties); err != nil {
	// 	return nil, err
	// }

	// partitions := sqlutil.PartitionOffsetStatements{}
	// if err = partitions.Prepare(db, d.writer, "account"); err != nil {
	// 	return nil, err
	// }
	var err error
	if err = d.accounts.prepare(d, serverName); err != nil {
		return nil, err
	}
	if err = d.profiles.prepare(d); err != nil {
		return nil, err
	}
	if err = d.accountDatas.prepare(d); err != nil {
		return nil, err
	}
	if err = d.threepids.prepare(d); err != nil {
		return nil, err
	}
	if err = d.openIDTokens.prepare(d, serverName); err != nil {
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
	// d.profilesMu.Lock()
	// defer d.profilesMu.Unlock()
	// return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// })
	return d.profiles.setAvatarURL(ctx, localpart, avatarURL)
}

// SetDisplayName updates the display name of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetDisplayName(
	ctx context.Context, localpart string, displayName string,
) error {
	// d.profilesMu.Lock()
	// defer d.profilesMu.Unlock()
	// return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// 	return d.profiles.setDisplayName(ctx, txn, localpart, displayName)
	// })
	return d.profiles.setDisplayName(ctx, localpart, displayName)
}

// SetPassword sets the account password to the given hash.
func (d *Database) SetPassword(
	ctx context.Context, localpart, plaintextPassword string,
) error {
	hash, err := d.hashPassword(plaintextPassword)
	if err != nil {
		return err
	}
	err = d.accounts.updatePassword(ctx, localpart, hash)
	return err
}

// CreateGuestAccount makes a new guest account and creates an empty profile
// for this account.
func (d *Database) CreateGuestAccount(ctx context.Context) (acc *api.Account, err error) {
	// We need to lock so we sequentially create numeric localparts. If we don't, two calls to
	// this function will cause the same number to be selected and one will fail with 'database is locked'
	// when the first txn upgrades to a write txn. We also need to lock the account creation else we can
	// race with CreateAccount
	// We know we'll be the only process since this is sqlite ;) so a lock here will be all that is needed.

	// d.profilesMu.Lock()
	// d.accountDatasMu.Lock()
	// d.accountsMu.Lock()
	// defer d.profilesMu.Unlock()
	// defer d.accountDatasMu.Unlock()
	// defer d.accountsMu.Unlock()
	// err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// })

	var numLocalpart int64
	numLocalpart, err = d.accounts.selectNewNumericLocalpart(ctx)
	if err != nil {
		return nil, err
	}
	localpart := strconv.FormatInt(numLocalpart, 10)
	acc, err = d.createAccount(ctx, localpart, "", "")
	return acc, err
}

// CreateAccount makes a new account with the given login name and password, and creates an empty profile
// for this account. If no password is supplied, the account will be a passwordless account. If the
// account already exists, it will return nil, ErrUserExists.
func (d *Database) CreateAccount(
	ctx context.Context, localpart, plaintextPassword, appserviceID string,
) (acc *api.Account, err error) {
	// Create one account at a time else we can get 'database is locked'.
	// d.profilesMu.Lock()
	// d.accountDatasMu.Lock()
	// d.accountsMu.Lock()
	// defer d.profilesMu.Unlock()
	// defer d.accountDatasMu.Unlock()
	// defer d.accountsMu.Unlock()
	// err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// 	acc, err = d.createAccount(ctx, txn, localpart, plaintextPassword, appserviceID)
	// 	return err
	// })

	acc, err = d.createAccount(ctx, localpart, plaintextPassword, appserviceID)
	return acc, err
}

// WARNING! This function assumes that the relevant mutexes have already
// been taken out by the caller (e.g. CreateAccount or CreateGuestAccount).
func (d *Database) createAccount(
	ctx context.Context, localpart, plaintextPassword, appserviceID string,
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
	if account, err = d.accounts.insertAccount(ctx, localpart, hash, appserviceID); err != nil {
		return nil, sqlutil.ErrUserExists
	}
	if err = d.profiles.insertProfile(ctx, localpart); err != nil {
		return nil, err
	}
	if err = d.accountDatas.insertAccountData(ctx, localpart, "", "m.push_rules", json.RawMessage(`{
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
	// d.accountDatasMu.Lock()
	// defer d.accountDatasMu.Unlock()
	// return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// })
	return d.accountDatas.insertAccountData(ctx, localpart, roomID, dataType, content)
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
	return d.accounts.selectNewNumericLocalpart(ctx)
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
	// d.threepidsMu.Lock()
	// defer d.threepidsMu.Unlock()
	// return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// })

	user, err := d.threepids.selectLocalpartForThreePID(
		ctx, threepid, medium,
	)
	if err != nil {
		return err
	}

	if len(user) > 0 {
		return Err3PIDInUse
	}

	return d.threepids.insertThreePID(ctx, threepid, medium, localpart)
}

// RemoveThreePIDAssociation removes the association involving a given third-party
// identifier.
// If no association exists involving this third-party identifier, returns nothing.
// If there was a problem talking to the database, returns an error.
func (d *Database) RemoveThreePIDAssociation(
	ctx context.Context, threepid string, medium string,
) (err error) {
	// d.threepidsMu.Lock()
	// defer d.threepidsMu.Unlock()
	// return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// })
	return d.threepids.deleteThreePID(ctx, threepid, medium)
}

// GetLocalpartForThreePID looks up the localpart associated with a given third-party
// identifier.
// If no association involves the given third-party idenfitier, returns an empty
// string.
// Returns an error if there was a problem talking to the database.
func (d *Database) GetLocalpartForThreePID(
	ctx context.Context, threepid string, medium string,
) (localpart string, err error) {
	return d.threepids.selectLocalpartForThreePID(ctx, threepid, medium)
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
	response, err := d.accounts.selectAccountByLocalpart(ctx, localpart)
	// if err == sql.ErrNoRows {
	// 	return true, nil
	// }
	return response == nil, err
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
	return d.accounts.deactivateAccount(ctx, localpart)
}

// CreateOpenIDToken persists a new token that was issued for OpenID Connect
func (d *Database) CreateOpenIDToken(
	ctx context.Context,
	token, localpart string,
) (int64, error) {
	expiresAtMS := time.Now().UnixNano()/int64(time.Millisecond) + d.openIDTokenLifetimeMS
	// err := d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
	// })
	var err = d.openIDTokens.insertToken(ctx, token, localpart, expiresAtMS)
	return expiresAtMS, err
}

// GetOpenIDTokenAttributes gets the attributes of issued an OIDC auth token
func (d *Database) GetOpenIDTokenAttributes(
	ctx context.Context,
	token string,
) (*api.OpenIDTokenAttributes, error) {
	return d.openIDTokens.selectOpenIDTokenAtrributes(ctx, token)
}
