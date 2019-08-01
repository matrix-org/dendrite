// Copyright 2019 Alex Chen
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
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
)

const redactionsSchema = `
-- The redactions table holds redactions.
CREATE TABLE IF NOT EXISTS syncapi_redactions (
    -- The event ID for the redaction event.
    event_id TEXT NOT NULL,
    -- The event ID for the redacted event.
    redacts TEXT NOT NULL,
	-- Whether the redaction has been validated.
	-- For use of the "accept first, validate later" strategy for rooms >= v3.
	-- Should always be TRUE for rooms before v3.
	validated BOOLEAN NOT NULL
);

CREATE INDEX IF NOT EXISTS syncapi_redactions_redacts ON syncapi_redactions(redacts);
`

const insertRedactionSQL = "" +
	"INSERT INTO syncapi_redactions (event_id, redacts, validated)" +
	" VALUES ($1, $2, $3)"

const bulkSelectRedactionSQL = "" +
	"SELECT event_id, redacts, validated FROM syncapi_redactions" +
	" WHERE redacts = ANY($1)"

const bulkUpdateValidationStatusSQL = "" +
	" UPDATE syncapi_redactions SET validated = $2 WHERE event_id = ANY($1)"

type redactionStatements struct {
	insertRedactionStmt            *sql.Stmt
	bulkSelectRedactionStmt        *sql.Stmt
	bulkUpdateValidationStatusStmt *sql.Stmt
}

// redactedToRedactionMap is a map in the form map[redactedEventID]redactionEventID.
type redactedToRedactionMap map[string]string

func (s *redactionStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(redactionsSchema)
	if err != nil {
		return
	}
	if s.insertRedactionStmt, err = db.Prepare(insertRedactionSQL); err != nil {
		return
	}
	if s.bulkSelectRedactionStmt, err = db.Prepare(bulkSelectRedactionSQL); err != nil {
		return
	}
	if s.bulkUpdateValidationStatusStmt, err = db.Prepare(bulkUpdateValidationStatusSQL); err != nil {
		return
	}
	return
}

func (s *redactionStatements) insertRedaction(
	ctx context.Context,
	txn *sql.Tx,
	eventID string,
	redactsEventID string,
	validated bool,
) error {
	stmt := common.TxStmt(txn, s.insertRedactionStmt)
	_, err := stmt.ExecContext(ctx, eventID, redactsEventID, validated)
	return err
}

// bulkSelectRedaction returns the redactions for the given event IDs.
func (s *redactionStatements) bulkSelectRedaction(
	ctx context.Context,
	txn *sql.Tx,
	eventIDs []string,
) (
	validated redactedToRedactionMap,
	unvalidated redactedToRedactionMap,
	err error,
) {
	stmt := common.TxStmt(txn, s.bulkSelectRedactionStmt)
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close() // nolint: errcheck

	validated = make(redactedToRedactionMap)
	unvalidated = make(redactedToRedactionMap)

	var (
		redactedByID    string
		redactedEventID string
		isValidated     bool
	)
	for rows.Next() {
		if err = rows.Scan(
			&redactedByID,
			&redactedEventID,
			&isValidated,
		); err != nil {
			return nil, nil, err
		}
		if isValidated {
			validated[redactedEventID] = redactedByID
		} else {
			unvalidated[redactedEventID] = redactedByID
		}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	return validated, unvalidated, nil
}

func (s *redactionStatements) bulkUpdateValidationStatus(
	ctx context.Context,
	txn *sql.Tx,
	eventIDs []string,
	newStatus bool,
) error {
	stmt := common.TxStmt(txn, s.bulkUpdateValidationStatusStmt)
	_, err := stmt.ExecContext(ctx, pq.StringArray(eventIDs), newStatus)
	return err
}
