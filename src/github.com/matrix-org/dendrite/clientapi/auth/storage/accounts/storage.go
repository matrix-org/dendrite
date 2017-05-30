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
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
)

// Database represents an account database
type Database struct {
	db       *sql.DB
	accounts accountsStatements
}

// NewDatabase creates a new accounts database
func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (*Database, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	a := accountsStatements{}
	if err = a.prepare(db, serverName); err != nil {
		return nil, err
	}
	return &Database{db, a}, nil
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

// CreateAccount makes a new account with the given login name and password. If no password is supplied,
// the account will be a passwordless account.
func (d *Database) CreateAccount(localpart, plaintextPassword string) (*authtypes.Account, error) {
	hash, err := hashPassword(plaintextPassword)
	if err != nil {
		return nil, err
	}
	return d.accounts.insertAccount(localpart, hash)
}

func hashPassword(plaintext string) (hash string, err error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	return string(hashBytes), err
}
