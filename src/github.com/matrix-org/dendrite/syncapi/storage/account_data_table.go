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

package storage

import (
	"database/sql"

	"github.com/matrix-org/dendrite/syncapi/types"
)

const accountDataSchema = `
-- Stores the users account data
CREATE TABLE IF NOT EXISTS account_data_type (
    -- The highest numeric ID from the output_room_events at the time of saving the data
    id BIGINT,
    -- ID of the user the data belongs to
    user_id TEXT NOT NULL,
	-- ID of the room the data is related to (empty string if not related to a specific room)
	room_id TEXT NOT NULL,
    -- Type of the data
    type TEXT NOT NULL,

	PRIMARY KEY(user_id, room_id, type),

    -- We don't want two entries of the same type for the same user
    CONSTRAINT account_data_unique UNIQUE (user_id, room_id, type)
);
`

const insertAccountDataSQL = "" +
	"INSERT INTO account_data_type (id, user_id, room_id, type) VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT ON CONSTRAINT account_data_unique" +
	" DO UPDATE SET id = EXCLUDED.id"

const selectAccountDataInRangeSQL = "" +
	"SELECT room_id, type FROM account_data_type" +
	" WHERE user_id = $1 AND id > $2 AND id <= $3" +
	" ORDER BY id ASC"

type accountDataStatements struct {
	insertAccountDataStmt        *sql.Stmt
	selectAccountDataInRangeStmt *sql.Stmt
}

func (s *accountDataStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(accountDataSchema)
	if err != nil {
		return
	}
	if s.insertAccountDataStmt, err = db.Prepare(insertAccountDataSQL); err != nil {
		return
	}
	if s.selectAccountDataInRangeStmt, err = db.Prepare(selectAccountDataInRangeSQL); err != nil {
		return
	}
	return
}

func (s *accountDataStatements) insertAccountData(
	pos types.StreamPosition, userID string, roomID string, dataType string,
) (err error) {
	_, err = s.insertAccountDataStmt.Exec(pos, userID, roomID, dataType)
	return
}

func (s *accountDataStatements) selectAccountDataInRange(
	userID string, oldPos types.StreamPosition, newPos types.StreamPosition,
) (data map[string][]string, err error) {
	data = make(map[string][]string)

	// If both positions are the same, it means that the data was saved after the
	// latest room event. In that case, we need to decrement the old position as
	// it would prevent the SQL request from returning anything.
	if oldPos == newPos {
		oldPos--
	}

	rows, err := s.selectAccountDataInRangeStmt.Query(userID, oldPos, newPos)
	if err != nil {
		return
	}

	for rows.Next() {
		var dataType string
		var roomID string

		if err = rows.Scan(&roomID, &dataType); err != nil {
			return
		}

		if len(data[roomID]) > 0 {
			data[roomID] = append(data[roomID], dataType)
		} else {
			data[roomID] = []string{dataType}
		}
	}

	return
}
