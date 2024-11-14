// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2017, 2018 New Vector Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/storage/sqlite3/deltas"
	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/element-hq/dendrite/roomserver/types"
)

// TODO: previous_reference_sha256 was NOT NULL before but it broke sytest because
// sytest sends no SHA256 sums in the prev_events references in the soft-fail tests.
// In Postgres an empty BYTEA field is not NULL so it's fine there. In SQLite it
// seems to care that it's empty and therefore hits a NOT NULL constraint on insert.
// We should really work out what the right thing to do here is.
const previousEventSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_previous_events (
    previous_event_id TEXT NOT NULL,
    event_nids TEXT NOT NULL,
    UNIQUE (previous_event_id)
  );
`

// Insert an entry into the previous_events table.
// If there is already an entry indicating that an event references that previous event then
// add the event NID to the list to indicate that this event references that previous event as well.
// This should only be modified while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
// The lock is necessary to avoid data races when checking whether an event is already referenced by another event.
const insertPreviousEventSQL = `
	INSERT OR REPLACE INTO roomserver_previous_events
	  (previous_event_id, event_nids)
	  VALUES ($1, $2)
`

const selectPreviousEventNIDsSQL = `
	SELECT event_nids FROM roomserver_previous_events
	  WHERE previous_event_id = $1
`

// Check if the event is referenced by another event in the table.
// This should only be done while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
const selectPreviousEventExistsSQL = `
	SELECT 1 FROM roomserver_previous_events
	  WHERE previous_event_id = $1
`

type previousEventStatements struct {
	db                            *sql.DB
	insertPreviousEventStmt       *sql.Stmt
	selectPreviousEventNIDsStmt   *sql.Stmt
	selectPreviousEventExistsStmt *sql.Stmt
}

func CreatePrevEventsTable(db *sql.DB) error {
	_, err := db.Exec(previousEventSchema)
	if err != nil {
		return err
	}
	// check if the column exists
	var cName string
	migrationName := "roomserver: drop column reference_sha from roomserver_prev_events"
	err = db.QueryRowContext(context.Background(), `SELECT p.name FROM sqlite_master AS m JOIN pragma_table_info(m.name) AS p WHERE m.name = 'roomserver_previous_events' AND p.name = 'previous_reference_sha256'`).Scan(&cName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { // migration was already executed, as the column was removed
			if err = sqlutil.InsertMigration(context.Background(), db, migrationName); err != nil {
				return fmt.Errorf("unable to manually insert migration '%s': %w", migrationName, err)
			}
			return nil
		}
		return err
	}
	m := sqlutil.NewMigrator(db)
	m.AddMigrations([]sqlutil.Migration{
		{
			Version: migrationName,
			Up:      deltas.UpDropEventReferenceSHAPrevEvents,
		},
	}...)
	return m.Up(context.Background())
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
	eventNID types.EventNID,
) error {
	var eventNIDs string
	eventNIDAsString := fmt.Sprintf("%d", eventNID)
	selectStmt := sqlutil.TxStmt(txn, s.selectPreviousEventExistsStmt)
	err := selectStmt.QueryRowContext(ctx, previousEventID).Scan(&eventNIDs)
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
		ctx, previousEventID, eventNIDs,
	)
	return err
}

// Check if the event reference exists
// Returns sql.ErrNoRows if the event reference doesn't exist.
func (s *previousEventStatements) SelectPreviousEventExists(
	ctx context.Context, txn *sql.Tx, eventID string,
) error {
	var ok int64
	stmt := sqlutil.TxStmt(txn, s.selectPreviousEventExistsStmt)
	return stmt.QueryRowContext(ctx, eventID).Scan(&ok)
}
