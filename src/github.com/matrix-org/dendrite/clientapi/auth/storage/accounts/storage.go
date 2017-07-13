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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
)

// Database represents an account database
type Database struct {
	db          *sql.DB
	partitions  common.PartitionOffsetStatements
	accounts    accountsStatements
	profiles    profilesStatements
	memberships membershipStatements
}

// NewDatabase creates a new accounts and profiles database
func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (*Database, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	partitions := common.PartitionOffsetStatements{}
	if err = partitions.Prepare(db); err != nil {
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
	return &Database{db, partitions, a, p, m}, nil
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
// room. It also stores the ID of the `join` membership event.
// If a membership already exists between the user and the room, or of the
// insert fails, returns the SQL error
func (d *Database) SaveMembership(localpart string, roomID string, eventID string) error {
	return d.memberships.insertMembership(localpart, roomID, eventID)
}

// RemoveMembership removes the membership of which the `join` membership event
// ID matches with the given event ID.
// If the removal fails, or if there is no membership to remove, returns an error
func (d *Database) RemoveMembership(eventID string) error {
	return d.memberships.deleteMembershipByEventID(eventID)
}

// GetMembershipByEventID returns the membership (as a user localpart and a room ID)
// for which the `join` membership event ID matches a given event ID
// If no membership match this event ID, the localpart and room ID will be empty strings
// If an error happens during the retrieval, returns the SQL error
func (d *Database) GetMembershipByEventID(eventID string) (string, string, error) {
	return d.memberships.selectMembershipByEventID(eventID)
}

func hashPassword(plaintext string) (hash string, err error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	return string(hashBytes), err
}
