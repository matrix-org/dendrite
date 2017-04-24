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
)

const mediaSchema = `
`

const insertMediaSQL = "" +
	"INSERT INTO events (room_nid, event_type_nid, event_state_key_nid, event_id, reference_sha256, auth_event_nids)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT ON CONSTRAINT event_id_unique" +
	" DO NOTHING" +
	" RETURNING event_nid, state_snapshot_nid"

type mediaStatements struct {
	insertMediaStmt *sql.Stmt
}

func (s *mediaStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(mediaSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertMediaStmt, insertMediaSQL},
	}.prepare(db)
}

func (s *mediaStatements) insertMedia() {
}
