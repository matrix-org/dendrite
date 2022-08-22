// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	"strings"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

// TODO: previous_reference_sha256 was NOT NULL before but it broke sytest because
// sytest sends no SHA256 sums in the prev_events references in the soft-fail tests.
// In Postgres an empty BYTEA field is not NULL so it's fine there. In SQLite it
// seems to care that it's empty and therefore hits a NOT NULL constraint on insert.
// We should really work out what the right thing to do here is.
const previousEventSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_previous_events (
    previous_event_id TEXT NOT NULL,
    previous_reference_sha256 BLOB,
    event_nids TEXT NOT NULL,
    UNIQUE (previous_event_id, previous_reference_sha256)
  );
`

// Insert an entry into the previous_events table.
// If there is already an entry indicating that an event references that previous event then
// add the event NID to the list to indicate that this event references that previous event as well.
// This should only be modified while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
// The lock is necessary to avoid data races when checking whether an event is already referenced by another event.
const insertPreviousEventSQL = `
	INSERT OR REPLACE INTO roomserver_previous_events
	  (previous_event_id, previous_reference_sha256, event_nids)
	  VALUES ($1, $2, $3)
`

const selectPreviousEventNIDsSQL = `
	SELECT event_nids FROM roomserver_previous_events
	  WHERE previous_event_id = $1 AND previous_reference_sha256 = $2
`

// Check if the event is referenced by another event in the table.
// This should only be done while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
const selectPreviousEventExistsSQL = `
	SELECT 1 FROM roomserver_previous_events
	  WHERE previous_event_id = $1 AND previous_reference_sha256 = $2
`

type previousEventStatements struct {
	db                            *sql.DB
	insertPreviousEventStmt       *sql.Stmt
	selectPreviousEventNIDsStmt   *sql.Stmt
	selectPreviousEventExistsStmt *sql.Stmt
}

func CreatePrevEventsTable(db *sql.DB) error {
	_, err := db.Exec(previousEventSchema)
	return err
}

func PreparePrevEventsTable(db *sql.DB) (tables.PreviousEvents, error) {
	s := &previousEventStatements{
		db: db,
	}

	return s, sqlutil.StatementList{
		{&s.insertPreviousEventStmt, insertPreviousEventSQL},
		{&s.selectPreviousEventNIDsStmt, selectPreviousEventNIDsSQL},
		{&s.selectPreviousEventExistsStmt, selectPreviousEventExistsSQL},
	}.Prepare(db)
}

func (s *previousEventStatements) InsertPreviousEvent(
	ctx context.Context,
	txn *sql.Tx,
	previousEventID string,
	previousEventReferenceSHA256 []byte,
	eventNID types.EventNID,
) error {
	var eventNIDs string
	eventNIDAsString := fmt.Sprintf("%d", eventNID)
	selectStmt := sqlutil.TxStmt(txn, s.selectPreviousEventExistsStmt)
	err := selectStmt.QueryRowContext(ctx, previousEventID, previousEventReferenceSHA256).Scan(&eventNIDs)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("selectStmt.QueryRowContext.Scan: %w", err)
	}
	var nids []string
	if eventNIDs != "" {
		nids = strings.Split(eventNIDs, ",")
		for _, nid := range nids {
			if nid == eventNIDAsString {
				return nil
			}
		}
		eventNIDs = strings.Join(append(nids, eventNIDAsString), ",")
	} else {
		eventNIDs = eventNIDAsString
	}
	insertStmt := sqlutil.TxStmt(txn, s.insertPreviousEventStmt)
	_, err = insertStmt.ExecContext(
		ctx, previousEventID, previousEventReferenceSHA256, eventNIDs,
	)
	return err
}

// Check if the event reference exists
// Returns sql.ErrNoRows if the event reference doesn't exist.
func (s *previousEventStatements) SelectPreviousEventExists(
	ctx context.Context, txn *sql.Tx, eventID string, eventReferenceSHA256 []byte,
) error {
	var ok int64
	stmt := sqlutil.TxStmt(txn, s.selectPreviousEventExistsStmt)
	return stmt.QueryRowContext(ctx, eventID, eventReferenceSHA256).Scan(&ok)
}
