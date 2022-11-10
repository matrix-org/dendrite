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

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

const profilesSchema = `
-- Stores data about accounts profiles.
CREATE TABLE IF NOT EXISTS userapi_profiles (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL,
	server_name TEXT NOT NULL,
    -- The display name for this account
    display_name TEXT,
    -- The URL of the avatar for this account
    avatar_url TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS userapi_profiles_idx ON userapi_profiles(localpart, server_name);
`

const insertProfileSQL = "" +
	"INSERT INTO userapi_profiles(localpart, server_name, display_name, avatar_url) VALUES ($1, $2, $3, $4)"

const selectProfileByLocalpartSQL = "" +
	"SELECT localpart, server_name, display_name, avatar_url FROM userapi_profiles WHERE localpart = $1 AND server_name = $2"

const setAvatarURLSQL = "" +
	"UPDATE userapi_profiles AS new" +
	" SET avatar_url = $1" +
	" FROM userapi_profiles AS old" +
	" WHERE new.localpart = $2 AND new.server_name = $3" +
	" RETURNING new.display_name, old.avatar_url <> new.avatar_url"

const setDisplayNameSQL = "" +
	"UPDATE userapi_profiles AS new" +
	" SET display_name = $1" +
	" FROM userapi_profiles AS old" +
	" WHERE new.localpart = $2 AND new.server_name = $3" +
	" RETURNING new.avatar_url, old.display_name <> new.display_name"

const selectProfilesBySearchSQL = "" +
	"SELECT localpart, server_name, display_name, avatar_url FROM userapi_profiles WHERE localpart LIKE $1 OR display_name LIKE $1 LIMIT $2"

type profilesStatements struct {
	serverNoticesLocalpart       string
	insertProfileStmt            *sql.Stmt
	selectProfileByLocalpartStmt *sql.Stmt
	setAvatarURLStmt             *sql.Stmt
	setDisplayNameStmt           *sql.Stmt
	selectProfilesBySearchStmt   *sql.Stmt
}

func NewPostgresProfilesTable(db *sql.DB, serverNoticesLocalpart string) (tables.ProfileTable, error) {
	s := &profilesStatements{
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
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.insertProfileStmt).ExecContext(ctx, localpart, serverName, "", "")
	return
}

func (s *profilesStatements) SelectProfileByLocalpart(
	ctx context.Context,
	localpart string, serverName gomatrixserverlib.ServerName,
) (*authtypes.Profile, error) {
	var profile authtypes.Profile
	err := s.selectProfileByLocalpartStmt.QueryRowContext(ctx, localpart, serverName).Scan(
		&profile.Localpart, &profile.ServerName, &profile.DisplayName, &profile.AvatarURL,
	)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *profilesStatements) SetAvatarURL(
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
	avatarURL string,
) (*authtypes.Profile, bool, error) {
	profile := &authtypes.Profile{
		Localpart:  localpart,
		ServerName: string(serverName),
		AvatarURL:  avatarURL,
	}
	var changed bool
	stmt := sqlutil.TxStmt(txn, s.setAvatarURLStmt)
	err := stmt.QueryRowContext(ctx, avatarURL, localpart, serverName).Scan(&profile.DisplayName, &changed)
	return profile, changed, err
}

func (s *profilesStatements) SetDisplayName(
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
	displayName string,
) (*authtypes.Profile, bool, error) {
	profile := &authtypes.Profile{
		Localpart:   localpart,
		ServerName:  string(serverName),
		DisplayName: displayName,
	}
	var changed bool
	stmt := sqlutil.TxStmt(txn, s.setDisplayNameStmt)
	err := stmt.QueryRowContext(ctx, displayName, localpart, serverName).Scan(&profile.AvatarURL, &changed)
	return profile, changed, err
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
		if err := rows.Scan(&profile.Localpart, &profile.ServerName, &profile.DisplayName, &profile.AvatarURL); err != nil {
			return nil, err
		}
		if profile.Localpart != s.serverNoticesLocalpart {
			profiles = append(profiles, profile)
		}
	}
	return profiles, nil
}
