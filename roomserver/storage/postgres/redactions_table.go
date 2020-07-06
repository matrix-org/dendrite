// Copyright 2020 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

const redactionsSchema = `
-- Stores information about the redacted state of events.
-- We need to track redactions rather than blindly updating the event JSON table on receipt of a redaction
-- because we might receive the redaction BEFORE we receive the event which it redacts (think backfill).
CREATE TABLE IF NOT EXISTS roomserver_redactions (
    redaction_event_id TEXT PRIMARY KEY,
	redacts_event_id TEXT NOT NULL,
	-- Initially FALSE, set to TRUE when the redaction has been validated according to rooms v3+ spec
	-- https://matrix.org/docs/spec/rooms/v3#authorization-rules-for-events
	validated BOOLEAN NOT NULL
);
CREATE INDEX IF NOT EXISTS roomserver_redactions_redacts_event_id ON roomserver_redactions(redacts_event_id);
`

const insertRedactionSQL = "" +
	"INSERT INTO roomserver_redactions (redaction_event_id, redacts_event_id, validated)" +
	" VALUES ($1, $2, $3)"

const selectRedactedEventSQL = "" +
	"SELECT redaction_event_id, redacts_event_id, validated FROM roomserver_redactions" +
	" WHERE redaction_event_id = $1"

const selectRedactionEventSQL = "" +
	"SELECT redaction_event_id, redacts_event_id, validated FROM roomserver_redactions" +
	" WHERE redacts_event_id = $1"

const markRedactionValidatedSQL = "" +
	" UPDATE roomserver_redactions SET validated = $2 WHERE redaction_event_id = $1"

type redactionStatements struct {
	insertRedactionStmt        *sql.Stmt
	selectRedactedEventStmt    *sql.Stmt
	selectRedactionEventStmt   *sql.Stmt
	markRedactionValidatedStmt *sql.Stmt
}

func NewPostgresRedactionsTable(db *sql.DB) (tables.Redactions, error) {
	s := &redactionStatements{}
	_, err := db.Exec(redactionsSchema)
	if err != nil {
		return nil, err
	}

	return s, shared.StatementList{
		{&s.insertRedactionStmt, insertRedactionSQL},
		{&s.selectRedactedEventStmt, selectRedactedEventSQL},
		{&s.selectRedactionEventStmt, selectRedactionEventSQL},
		{&s.markRedactionValidatedStmt, markRedactionValidatedSQL},
	}.Prepare(db)
}

func (s *redactionStatements) InsertRedaction(
	ctx context.Context, txn *sql.Tx, info tables.RedactionInfo,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertRedactionStmt)
	_, err := stmt.ExecContext(ctx, info.RedactionEventID, info.RedactsEventID, info.Validated)
	return err
}

func (s *redactionStatements) SelectRedactedEvent(
	ctx context.Context, txn *sql.Tx, redactionEventID string,
) (info *tables.RedactionInfo, err error) {
	info = &tables.RedactionInfo{}
	stmt := sqlutil.TxStmt(txn, s.selectRedactedEventStmt)
	err = stmt.QueryRowContext(ctx, redactionEventID).Scan(
		&info.RedactionEventID, &info.RedactsEventID, &info.Validated,
	)
	if err == sql.ErrNoRows {
		err = nil
		info = nil
	}
	return
}

func (s *redactionStatements) SelectRedactionEvent(
	ctx context.Context, txn *sql.Tx, redactedEventID string,
) (info *tables.RedactionInfo, err error) {
	info = &tables.RedactionInfo{}
	stmt := sqlutil.TxStmt(txn, s.selectRedactionEventStmt)
	err = stmt.QueryRowContext(ctx, redactedEventID).Scan(
		&info.RedactionEventID, &info.RedactsEventID, &info.Validated,
	)
	if err == sql.ErrNoRows {
		err = nil
		info = nil
	}
	return
}

func (s *redactionStatements) MarkRedactionValidated(
	ctx context.Context, txn *sql.Tx, redactionEventID string, validated bool,
) error {
	stmt := sqlutil.TxStmt(txn, s.markRedactionValidatedStmt)
	_, err := stmt.ExecContext(ctx, redactionEventID, validated)
	return err
}
