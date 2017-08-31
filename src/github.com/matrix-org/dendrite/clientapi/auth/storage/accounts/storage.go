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

package accounts

import (
	"database/sql"
	"errors"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
)

// Database represents an account database
type Database struct {
	db           *sql.DB
	partitions   common.PartitionOffsetStatements
	accounts     accountsStatements
	profiles     profilesStatements
	memberships  membershipStatements
	accountDatas accountDataStatements
	threepids    threepidStatements
	serverName   gomatrixserverlib.ServerName
}

// NewDatabase creates a new accounts and profiles database
func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (*Database, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
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
	return &Database{db, partitions, a, p, m, ac, t, serverName}, nil
}

// GetAccountByPassword returns the account associated with the given localpart and password.
// Returns sql.ErrNoRows if no account exists which matches the given localpart.
func (d *Database) GetAccountByPassword(localpart, plaintextPassword string) (*authtypes.Account, error) {
	hash, err := d.accounts.selectPasswordHash(localpart)
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintextPassword)); err != nil {
		return nil, err
	}
	return d.accounts.selectAccountByLocalpart(localpart)
}

// GetProfileByLocalpart returns the profile associated with the given localpart.
// Returns sql.ErrNoRows if no profile exists which matches the given localpart.
func (d *Database) GetProfileByLocalpart(localpart string) (*authtypes.Profile, error) {
	return d.profiles.selectProfileByLocalpart(localpart)
}

// SetAvatarURL updates the avatar URL of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetAvatarURL(localpart string, avatarURL string) error {
	return d.profiles.setAvatarURL(localpart, avatarURL)
}

// SetDisplayName updates the display name of the profile associated with the given
// localpart. Returns an error if something went wrong with the SQL query
func (d *Database) SetDisplayName(localpart string, displayName string) error {
	return d.profiles.setDisplayName(localpart, displayName)
}

// CreateAccount makes a new account with the given login name and password, and creates an empty profile
// for this account. If no password is supplied, the account will be a passwordless account.
func (d *Database) CreateAccount(localpart, plaintextPassword string) (*authtypes.Account, error) {
	hash, err := hashPassword(plaintextPassword)
	if err != nil {
		return nil, err
	}
	if err := d.profiles.insertProfile(localpart); err != nil {
		return nil, err
	}
	return d.accounts.insertAccount(localpart, hash)
}

// PartitionOffsets implements common.PartitionStorer
func (d *Database) PartitionOffsets(topic string) ([]common.PartitionOffset, error) {
	return d.partitions.SelectPartitionOffsets(topic)
}

// SetPartitionOffset implements common.PartitionStorer
func (d *Database) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.partitions.UpsertPartitionOffset(topic, partition, offset)
}

// SaveMembership saves the user matching a given localpart as a member of a given
// room. It also stores the ID of the membership event and a flag on whether the user
// is still in the room.
// If a membership already exists between the user and the room, or of the
// insert fails, returns the SQL error
func (d *Database) SaveMembership(localpart string, roomID string, eventID string, txn *sql.Tx) error {
	return d.memberships.insertMembership(localpart, roomID, eventID, txn)
}

// removeMembershipsByEventIDs removes the memberships of which the `join` membership
// event ID is included in a given array of events IDs
// If the removal fails, or if there is no membership to remove, returns an error
func (d *Database) removeMembershipsByEventIDs(eventIDs []string, txn *sql.Tx) error {
	return d.memberships.deleteMembershipsByEventIDs(eventIDs, txn)
}

// UpdateMemberships adds the "join" membership events included in a given state
// events array, and removes those which ID is included in a given array of events
// IDs. All of the process is run in a transaction, which commits only once/if every
// insertion and deletion has been successfully processed.
// Returns a SQL error if there was an issue with any part of the process
func (d *Database) UpdateMemberships(eventsToAdd []gomatrixserverlib.Event, idsToRemove []string) error {
	return common.WithTransaction(d.db, func(txn *sql.Tx) error {
		if err := d.removeMembershipsByEventIDs(idsToRemove, txn); err != nil {
			return err
		}

		for _, event := range eventsToAdd {
			if err := d.newMembership(event, txn); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetMembershipsByLocalpart returns an array containing the memberships for all
// the rooms a user matching a given localpart is a member of
// If no membership match the given localpart, returns an empty array
// If there was an issue during the retrieval, returns the SQL error
func (d *Database) GetMembershipsByLocalpart(localpart string) (memberships []authtypes.Membership, err error) {
	return d.memberships.selectMembershipsByLocalpart(localpart)
}

// newMembership will save a new membership in the database, with a flag on whether
// the user is still in the room. This flag is set to true if the given state
// event is a "join" membership event and false if the event is a "leave" or "ban"
// membership. If the event isn't a m.room.member event with one of these three
// values, does nothing.
// If the event isn't a "join" membership event, does nothing
// If an error occurred, returns it
func (d *Database) newMembership(ev gomatrixserverlib.Event, txn *sql.Tx) error {
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
		if membership == "join" {
			if err := d.SaveMembership(localpart, roomID, eventID, txn); err != nil {
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
func (d *Database) SaveAccountData(localpart string, roomID string, dataType string, content string) error {
	return d.accountDatas.insertAccountData(localpart, roomID, dataType, content)
}

// GetAccountData returns account data related to a given localpart
// If no account data could be found, returns an empty arrays
// Returns an error if there was an issue with the retrieval
func (d *Database) GetAccountData(localpart string) (
	global []gomatrixserverlib.ClientEvent,
	rooms map[string][]gomatrixserverlib.ClientEvent,
	err error,
) {
	return d.accountDatas.selectAccountData(localpart)
}

// GetAccountDataByType returns account data matching a given
// localpart, room ID and type.
// If no account data could be found, returns an empty array
// Returns an error if there was an issue with the retrieval
func (d *Database) GetAccountDataByType(localpart string, roomID string, dataType string) (data []gomatrixserverlib.ClientEvent, err error) {
	return d.accountDatas.selectAccountDataByType(localpart, roomID, dataType)
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
func (d *Database) SaveThreePIDAssociation(threepid string, localpart string, medium string) (err error) {
	return common.WithTransaction(d.db, func(txn *sql.Tx) error {
		user, err := d.threepids.selectLocalpartForThreePID(txn, threepid, medium)
		if err != nil {
			return err
		}

		if len(user) > 0 {
			return Err3PIDInUse
		}

		return d.threepids.insertThreePID(txn, threepid, medium, localpart)
	})
}

// RemoveThreePIDAssociation removes the association involving a given third-party
// identifier.
// If no association exists involving this third-party identifier, returns nothing.
// If there was a problem talking to the database, returns an error.
func (d *Database) RemoveThreePIDAssociation(threepid string, medium string) (err error) {
	return d.threepids.deleteThreePID(threepid, medium)
}

// GetLocalpartForThreePID looks up the localpart associated with a given third-party
// identifier.
// If no association involves the given third-party idenfitier, returns an empty
// string.
// Returns an error if there was a problem talking to the database.
func (d *Database) GetLocalpartForThreePID(threepid string, medium string) (localpart string, err error) {
	return d.threepids.selectLocalpartForThreePID(nil, threepid, medium)
}

// GetThreePIDsForLocalpart looks up the third-party identifiers associated with
// a given local user.
// If no association is known for this user, returns an empty slice.
// Returns an error if there was an issue talking to the database.
func (d *Database) GetThreePIDsForLocalpart(localpart string) (threepids map[string]string, err error) {
	return d.threepids.selectThreePIDsForLocalpart(localpart)
}
