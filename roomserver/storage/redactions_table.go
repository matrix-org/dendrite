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
	"github.com/matrix-org/dendrite/roomserver/types"
)

const redactionsSchema = `
-- The redactions table holds redactions.
CREATE TABLE IF NOT EXISTS roomserver_redactions (
    -- Local numeric ID for the redaction event.
    event_nid BIGINT PRIMARY KEY,
    -- String ID for the redacted event.
    redacts TEXT NOT NULL,
	-- Whether the redaction has been validated.
	-- For use of the "accept first, validate later" strategy for rooms >= v3.
	-- Should always be TRUE for rooms before v3.
	validated BOOLEAN NOT NULL
);

CREATE INDEX IF NOT EXISTS roomserver_redactions_redacts ON roomserver_redactions(redacts);
`

const insertRedactionSQL = "" +
	"INSERT INTO roomserver_redactions (event_nid, redacts, validated)" +
	" VALUES ($1, $2, $3)"

const bulkSelectRedactionSQL = "" +
	"SELECT event_nid, redacts, validated FROM roomserver_redactions" +
	" WHERE redacts = ANY($1)"

const bulkUpdateValidationStatusSQL = "" +
	" UPDATE roomserver_redactions SET validated = $2 WHERE event_nid = ANY($1)"

type redactionStatements struct {
	insertRedactionStmt            *sql.Stmt
	bulkSelectRedactionStmt        *sql.Stmt
	bulkUpdateValidationStatusStmt *sql.Stmt
}

// redactedToRedactionMap is a map in the form map[redactedEventID]redactionEventNID.
type redactedToRedactionMap map[string]types.EventNID

func (s *redactionStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(redactionsSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertRedactionStmt, insertRedactionSQL},
		{&s.bulkSelectRedactionStmt, bulkSelectRedactionSQL},
		{&s.bulkUpdateValidationStatusStmt, bulkUpdateValidationStatusSQL},
	}.prepare(db)
}

func (s *redactionStatements) insertRedaction(
	ctx context.Context,
	txn *sql.Tx,
	eventNID types.EventNID,
	redactsEventID string,
	validated bool,
) error {
	stmt := common.TxStmt(txn, s.insertRedactionStmt)
	_, err := stmt.ExecContext(ctx, int64(eventNID), redactsEventID, validated)
	return err
}

// bulkSelectRedaction returns the redactions for the given event IDs.
// Return values validated and unvalidated are both map[redactedEventID]redactedByNID.
func (s *redactionStatements) bulkSelectRedaction(
	ctx context.Context,
	txn *sql.Tx,
	eventIDs []string,
) (
	validated, unvalidated redactedToRedactionMap,
	err error,
) {
	stmt := common.TxStmt(txn, s.bulkSelectRedactionStmt)
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close() // nolint: errcheck

	validated = make(map[string]types.EventNID)
	unvalidated = make(map[string]types.EventNID)

	var (
		redactedByNID   types.EventNID
		redactedEventID string
		isValidated     bool
	)
	for rows.Next() {
		if err = rows.Scan(
			&redactedByNID,
			&redactedEventID,
			&isValidated,
		); err != nil {
			return nil, nil, err
		}
		if isValidated {
			validated[redactedEventID] = redactedByNID
		} else {
			unvalidated[redactedEventID] = redactedByNID
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
	eventNIDs []types.EventNID,
	newStatus bool,
) error {
	stmt := common.TxStmt(txn, s.bulkUpdateValidationStatusStmt)
	_, err := stmt.ExecContext(ctx, eventNIDsAsArray(eventNIDs), newStatus)
	return err
}
