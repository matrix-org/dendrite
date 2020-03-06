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
	"errors"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"

	// Import the postgres database driver.
	_ "github.com/mattn/go-sqlite3"
)

// Database represents an account database
type Database struct {
	db *sql.DB
	common.PartitionOffsetStatements
	accounts     accountsStatements
	profiles     profilesStatements
	memberships  membershipStatements
	accountDatas accountDataStatements
	threepids    threepidStatements
	filter       filterStatements
	serverName   gomatrixserverlib.ServerName
}

// NewDatabase creates a new accounts and profiles database
func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (*Database, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open(common.SQLiteDriverName(), dataSourceName); err != nil {
		return nil, err
	}
	partitions := common.PartitionOffsetStatements{}
	if err = partitions.Prepare(db, "account"); err != nil {
		return nil, err
	}
	a := accountsStatements{}
	if err = a.prepare(db, serverName); err != nil {
		return nil, err
	}
	p := profilesStatements{}
	if err = p.prepare(db); err != nil {
		return nil, err
	}
	m := membershipStatements{}
	if err = m.prepare(db); err != nil {
		return nil, err
	}
	ac := accountDataStatements{}
	if err = ac.prepare(db); err != nil {
		return nil, err
	}
	t := threepidStatements{}
	if err = t.prepare(db); err != nil {
		return nil, err
	}
	f := filterStatements{}
	if err = f.prepare(db); err != nil {
		return nil, err
	}
	return &Database{db, partitions, a, p, m, ac, t, f, serverName}, nil
}

// GetAccountByPassword returns the account associated with the given localpart and password.
// Returns sql.ErrNoRows if no account exists which matches the given localpart.
func (d *Database) GetAccountByPassword(
	ctx context.Context, localpart, plaintextPassword string,
) (*authtypes.Account, error) {
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
	return d.profiles.setAvatarURL(ctx, localpart, avatarURL)
}

// SetDisplayName updates the display name of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetDisplayName(
	ctx context.Context, localpart string, displayName string,
) error {
	return d.profiles.setDisplayName(ctx, localpart, displayName)
}

// CreateGuestAccount makes a new guest account and creates an empty profile
// for this account.
func (d *Database) CreateGuestAccount(ctx context.Context) (acc *authtypes.Account, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
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
// account already exists, it will return nil, nil.
func (d *Database) CreateAccount(
	ctx context.Context, localpart, plaintextPassword, appserviceID string,
) (acc *authtypes.Account, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		acc, err = d.createAccount(ctx, txn, localpart, plaintextPassword, appserviceID)
		return err
	})
	return
}

func (d *Database) createAccount(
	ctx context.Context, txn *sql.Tx, localpart, plaintextPassword, appserviceID string,
) (*authtypes.Account, error) {
	var err error
	// Generate a password hash if this is not a password-less user
	hash := ""
	if plaintextPassword != "" {
		hash, err = hashPassword(plaintextPassword)
		if err != nil {
			return nil, err
		}
	}
	if err := d.profiles.insertProfile(ctx, txn, localpart); err != nil {
		if common.IsUniqueConstraintViolationErr(err) {
			return nil, nil
		}
		return nil, err
	}

	if err := d.accountDatas.insertAccountData(ctx, txn, localpart, "", "m.push_rules", `{
		"global": {
			"content": [],
			"override": [],
			"room": [],
			"sender": [],
			"underride": []
		}
	}`); err != nil {
		return nil, err
	}
	return d.accounts.insertAccount(ctx, txn, localpart, hash, appserviceID)
}

// SaveMembership saves the user matching a given localpart as a member of a given
// room. It also stores the ID of the membership event.
// If a membership already exists between the user and the room, or if the
// insert fails, returns the SQL error
func (d *Database) saveMembership(
	ctx context.Context, txn *sql.Tx, localpart, roomID, eventID string,
) error {
	return d.memberships.insertMembership(ctx, txn, localpart, roomID, eventID)
}

// removeMembershipsByEventIDs removes the memberships corresponding to the
// `join` membership events IDs in the eventIDs slice.
// If the removal fails, or if there is no membership to remove, returns an error
func (d *Database) removeMembershipsByEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) error {
	return d.memberships.deleteMembershipsByEventIDs(ctx, txn, eventIDs)
}

// UpdateMemberships adds the "join" membership events included in a given state
// events array, and removes those which ID is included in a given array of events
// IDs. All of the process is run in a transaction, which commits only once/if every
// insertion and deletion has been successfully processed.
// Returns a SQL error if there was an issue with any part of the process
func (d *Database) UpdateMemberships(
	ctx context.Context, eventsToAdd []gomatrixserverlib.Event, idsToRemove []string,
) error {
	return common.WithTransaction(d.db, func(txn *sql.Tx) error {
		if err := d.removeMembershipsByEventIDs(ctx, txn, idsToRemove); err != nil {
			return err
		}

		for _, event := range eventsToAdd {
			if err := d.newMembership(ctx, txn, event); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetMembershipInRoomByLocalpart returns the membership for an user
// matching the given localpart if he is a member of the room matching roomID,
// if not sql.ErrNoRows is returned.
// If there was an issue during the retrieval, returns the SQL error
func (d *Database) GetMembershipInRoomByLocalpart(
	ctx context.Context, localpart, roomID string,
) (authtypes.Membership, error) {
	return d.memberships.selectMembershipInRoomByLocalpart(ctx, localpart, roomID)
}

// GetMembershipsByLocalpart returns an array containing the memberships for all
// the rooms a user matching a given localpart is a member of
// If no membership match the given localpart, returns an empty array
// If there was an issue during the retrieval, returns the SQL error
func (d *Database) GetMembershipsByLocalpart(
	ctx context.Context, localpart string,
) (memberships []authtypes.Membership, err error) {
	return d.memberships.selectMembershipsByLocalpart(ctx, localpart)
}

// newMembership saves a new membership in the database.
// If the event isn't a valid m.room.member event with type `join`, does nothing.
// If an error occurred, returns the SQL error
func (d *Database) newMembership(
	ctx context.Context, txn *sql.Tx, ev gomatrixserverlib.Event,
) error {
	if ev.Type() == "m.room.member" && ev.StateKey() != nil {
		localpart, serverName, err := gomatrixserverlib.SplitID('@', *ev.StateKey())
		if err != nil {
			return err
		}

		// We only want state events from local users
		if string(serverName) != string(d.serverName) {
			return nil
		}

		eventID := ev.EventID()
		roomID := ev.RoomID()
		membership, err := ev.Membership()
		if err != nil {
			return err
		}

		// Only "join" membership events can be considered as new memberships
		if membership == gomatrixserverlib.Join {
			if err := d.saveMembership(ctx, txn, localpart, roomID, eventID); err != nil {
				return err
			}
		}
	}
	return nil
}

// SaveAccountData saves new account data for a given user and a given room.
// If the account data is not specific to a room, the room ID should be an empty string
// If an account data already exists for a given set (user, room, data type), it will
// update the corresponding row with the new content
// Returns a SQL error if there was an issue with the insertion/update
func (d *Database) SaveAccountData(
	ctx context.Context, localpart, roomID, dataType, content string,
) error {
	return common.WithTransaction(d.db, func(txn *sql.Tx) error {
		return d.accountDatas.insertAccountData(ctx, txn, localpart, roomID, dataType, content)
	})
}

// GetAccountData returns account data related to a given localpart
// If no account data could be found, returns an empty arrays
// Returns an error if there was an issue with the retrieval
func (d *Database) GetAccountData(ctx context.Context, localpart string) (
	global []gomatrixserverlib.ClientEvent,
	rooms map[string][]gomatrixserverlib.ClientEvent,
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
) (data *gomatrixserverlib.ClientEvent, err error) {
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

func hashPassword(plaintext string) (hash string, err error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
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
	return common.WithTransaction(d.db, func(txn *sql.Tx) error {
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

// GetFilter looks up the filter associated with a given local user and filter ID.
// Returns a filter structure. Otherwise returns an error if no such filter exists
// or if there was an error talking to the database.
func (d *Database) GetFilter(
	ctx context.Context, localpart string, filterID string,
) (*gomatrixserverlib.Filter, error) {
	return d.filter.selectFilter(ctx, localpart, filterID)
}

// PutFilter puts the passed filter into the database.
// Returns the filterID as a string. Otherwise returns an error if something
// goes wrong.
func (d *Database) PutFilter(
	ctx context.Context, localpart string, filter *gomatrixserverlib.Filter,
) (string, error) {
	return d.filter.insertFilter(ctx, filter, localpart)
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
) (*authtypes.Account, error) {
	return d.accounts.selectAccountByLocalpart(ctx, localpart)
}
