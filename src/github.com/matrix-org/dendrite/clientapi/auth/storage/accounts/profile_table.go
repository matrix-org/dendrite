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
)

const profilesSchema = `
-- Stores data about accounts profiles.
CREATE TABLE IF NOT EXISTS profiles (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL PRIMARY KEY,
    -- The display name for this account
    display_name TEXT,
    -- The URL of the avatar for this account
    avatar_url TEXT
);
`

const insertProfileSQL = "" +
	"INSERT INTO profiles(localpart, display_name, avatar_url) VALUES ($1, $2, $3)"

const selectProfileByLocalpartSQL = "" +
	"SELECT localpart, display_name, avatar_url FROM profiles WHERE localpart = $1"

const setAvatarURLSQL = "" +
	"UPDATE profiles SET avatar_url = $1 WHERE localpart = $2"

const setDisplayNameSQL = "" +
	"UPDATE profiles SET display_name = $1 WHERE localpart = $2"

type profilesStatements struct {
	insertProfileStmt            *sql.Stmt
	selectProfileByLocalpartStmt *sql.Stmt
	setAvatarURLStmt             *sql.Stmt
	setDisplayNameStmt           *sql.Stmt
}

func (s *profilesStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(profilesSchema)
	if err != nil {
		return
	}
	if s.insertProfileStmt, err = db.Prepare(insertProfileSQL); err != nil {
		return
	}
	if s.selectProfileByLocalpartStmt, err = db.Prepare(selectProfileByLocalpartSQL); err != nil {
		return
	}
	if s.setAvatarURLStmt, err = db.Prepare(setAvatarURLSQL); err != nil {
		return
	}
	if s.setDisplayNameStmt, err = db.Prepare(setDisplayNameSQL); err != nil {
		return
	}
	return
}

func (s *profilesStatements) insertProfile(localpart string) (err error) {
	_, err = s.insertProfileStmt.Exec(localpart, "", "")
	return
}

func (s *profilesStatements) selectProfileByLocalpart(localpart string) (*authtypes.Profile, error) {
	var profile authtypes.Profile
	err := s.selectProfileByLocalpartStmt.QueryRow(localpart).Scan(&profile.Localpart, &profile.DisplayName, &profile.AvatarURL)
	return &profile, err
}

func (s *profilesStatements) setAvatarURL(localpart string, avatarURL string) (err error) {
	_, err = s.setAvatarURLStmt.Exec(avatarURL, localpart)
	return
}

func (s *profilesStatements) setDisplayName(localpart string, displayName string) (err error) {
	_, err = s.setDisplayNameStmt.Exec(displayName, localpart)
	return
}
