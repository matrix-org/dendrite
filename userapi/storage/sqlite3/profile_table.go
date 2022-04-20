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
	"fmt"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

const profilesSchema = `
-- Stores data about accounts profiles.
CREATE TABLE IF NOT EXISTS account_profiles (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL PRIMARY KEY,
    -- The display name for this account
    display_name TEXT,
    -- The URL of the avatar for this account
    avatar_url TEXT
);
`

const insertProfileSQL = "" +
	"INSERT INTO account_profiles(localpart, display_name, avatar_url) VALUES ($1, $2, $3)"

const selectProfileByLocalpartSQL = "" +
	"SELECT localpart, display_name, avatar_url FROM account_profiles WHERE localpart = $1"

const setAvatarURLSQL = "" +
	"UPDATE account_profiles SET avatar_url = $1 WHERE localpart = $2"

const setDisplayNameSQL = "" +
	"UPDATE account_profiles SET display_name = $1 WHERE localpart = $2"

const selectProfilesBySearchSQL = "" +
	"SELECT localpart, display_name, avatar_url FROM account_profiles WHERE localpart LIKE $1 OR display_name LIKE $1 LIMIT $2"

type profilesStatements struct {
	db                           *sql.DB
	serverNoticesLocalpart       string
	insertProfileStmt            *sql.Stmt
	selectProfileByLocalpartStmt *sql.Stmt
	setAvatarURLStmt             *sql.Stmt
	setDisplayNameStmt           *sql.Stmt
	selectProfilesBySearchStmt   *sql.Stmt
}

func NewSQLiteProfilesTable(db *sql.DB, serverNoticesLocalpart string) (tables.ProfileTable, error) {
	s := &profilesStatements{
		db:                     db,
		serverNoticesLocalpart: serverNoticesLocalpart,
	}
	_, err := db.Exec(profilesSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertProfileStmt, insertProfileSQL},
		{&s.selectProfileByLocalpartStmt, selectProfileByLocalpartSQL},
		{&s.setAvatarURLStmt, setAvatarURLSQL},
		{&s.setDisplayNameStmt, setDisplayNameSQL},
		{&s.selectProfilesBySearchStmt, selectProfilesBySearchSQL},
	}.Prepare(db)
}

func (s *profilesStatements) InsertProfile(
	ctx context.Context, txn *sql.Tx, localpart string,
) error {
	_, err := sqlutil.TxStmt(txn, s.insertProfileStmt).ExecContext(ctx, localpart, "", "")
	return err
}

func (s *profilesStatements) SelectProfileByLocalpart(
	ctx context.Context, localpart string,
) (*authtypes.Profile, error) {
	var profile authtypes.Profile
	err := s.selectProfileByLocalpartStmt.QueryRowContext(ctx, localpart).Scan(
		&profile.Localpart, &profile.DisplayName, &profile.AvatarURL,
	)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *profilesStatements) SetAvatarURL(
	ctx context.Context, txn *sql.Tx, localpart string, avatarURL string,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.setAvatarURLStmt)
	_, err = stmt.ExecContext(ctx, avatarURL, localpart)
	return
}

func (s *profilesStatements) SetDisplayName(
	ctx context.Context, txn *sql.Tx, localpart string, displayName string,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.setDisplayNameStmt)
	_, err = stmt.ExecContext(ctx, displayName, localpart)
	return
}

func (s *profilesStatements) SelectProfilesBySearch(
	ctx context.Context, searchString string, limit int,
) ([]authtypes.Profile, error) {
	var profiles []authtypes.Profile
	// The fmt.Sprintf directive below is building a parameter for the
	// "LIKE" condition in the SQL query. %% escapes the % char, so the
	// statement in the end will look like "LIKE %searchString%".
	rows, err := s.selectProfilesBySearchStmt.QueryContext(ctx, fmt.Sprintf("%%%s%%", searchString), limit)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectProfilesBySearch: rows.close() failed")
	for rows.Next() {
		var profile authtypes.Profile
		if err := rows.Scan(&profile.Localpart, &profile.DisplayName, &profile.AvatarURL); err != nil {
			return nil, err
		}
		if profile.Localpart != s.serverNoticesLocalpart {
			profiles = append(profiles, profile)
		}
	}
	return profiles, nil
}
