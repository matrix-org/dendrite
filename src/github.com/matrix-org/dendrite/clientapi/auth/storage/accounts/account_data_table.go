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
)

const accountDataSchema = `
-- Stores data about accounts profiles.
CREATE TABLE IF NOT EXISTS account_data (
    -- The Matrix user ID localpart for this account
    localpart TEXT NOT NULL,
    -- The room ID for this data (null if not specific to a room)
    room_id TEXT,
    -- The account data type
    type TEXT NOT NULL,
    -- The account data content
    content TEXT NOT NULL

    PRIMARY KEY(localpart, room_id, type)
);

-- Create index we can reference in the upsert request
CREATE UNIQUE INDEX IF NOT EXISTS ac_user_room_type ON account_data(localpart, room_id, type);
`

const insertAccountDataSQL = `
	INSERT INTO account_data(localpart, room_id, type, content) VALUES($1, $2, $3, $4)
	ON CONFLICT (ac_user_room_type) DO UPDATE SET content = EXCLUDED.content
`

const selectAccountDataByLocalPartSQL = "" +
	"SELECT room_id, type, content FROM account_data WHERE localpart = $1"

const deleteAccountDataSQL = "" +
	"DELETE FROM account_data WHERE localpart = $1 AND room_id = $2 AND type = $3"

type accountDataStatements struct {
	insertAccountDataStmt            *sql.Stmt
	selectAccountDataByLocalPartStmt *sql.Stmt
}

func (s *accountDataStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(accountsSchema)
	if err != nil {
		return
	}
	if s.insertAccountDataStmt, err = db.Prepare(insertAccountDataSQL); err != nil {
		return
	}
	if s.selectAccountDataByLocalPartStmt, err = db.Prepare(selectAccountDataByLocalPartSQL); err != nil {
		return
	}
	return
}

func (s *accountDataStatements) insertAccountData(localpart string, roomID string, dataType string, content string) (err error) {
	_, err = s.insertAccountDataStmt.Exec(localpart, roomID, dataType, content)
	return
}
