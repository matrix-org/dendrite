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
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const queueRetrySchema = `
-- The queue_retry table contains events that we failed to
-- send to a destination host, such that we can try them again
-- later. 
CREATE TABLE IF NOT EXISTS federationsender_queue_retry (
    -- The string ID of the room.
	transaction_id TEXT NOT NULL,
	-- The event type: "pdu", "invite", "send_to_device".
	send_type TEXT NOT NULL,
    -- The event ID of the m.room.member join event.
	event_id TEXT NOT NULL,
	-- The origin server TS of the event.
	origin_server_ts BIGINT NOT NULL,
    -- The domain part of the user ID the m.room.member event is for.
	server_name TEXT NOT NULL,
	-- The JSON body.
	json_body BYTEA NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_retry_event_id_idx
    ON federationsender_queue_retry (event_id);
`

const insertRetrySQL = "" +
	"INSERT INTO federationsender_queue_retry (transaction_id, send_type, event_id, origin_server_ts, server_name, json_body)" +
	" VALUES ($1, $2, $3, $4, $5, $6)"

const deleteRetrySQL = "" +
	"DELETE FROM federationsender_queue_retry WHERE event_id = ANY($1)"

const selectRetryNextTransactionIDSQL = "" +
	"SELECT transaction_id FROM federationsender_queue_retry" +
	" WHERE server_name = $1 AND send_type = $2" +
	" ORDER BY transaction_id ASC" +
	" LIMIT 1"

const selectRetryPDUsByTransactionSQL = "" +
	"SELECT event_id, server_name, origin_server_ts, json_body FROM federationsender_queue_retry" +
	" WHERE server_name = $1 AND send_type = $2 AND transaction_id = $3" +
	" LIMIT 50"

type queueRetryStatements struct {
	insertRetryStmt                  *sql.Stmt
	deleteRetryStmt                  *sql.Stmt
	selectRetryNextTransactionIDStmt *sql.Stmt
	selectRetryPDUsByTransactionStmt *sql.Stmt
}

func (s *queueRetryStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(queueRetrySchema)
	if err != nil {
		return
	}
	if s.insertRetryStmt, err = db.Prepare(insertRetrySQL); err != nil {
		return
	}
	if s.deleteRetryStmt, err = db.Prepare(deleteRetrySQL); err != nil {
		return
	}
	if s.selectRetryNextTransactionIDStmt, err = db.Prepare(selectRetryNextTransactionIDSQL); err != nil {
		return
	}
	if s.selectRetryPDUsByTransactionStmt, err = db.Prepare(selectRetryPDUsByTransactionSQL); err != nil {
		return
	}
	return
}

func (s *queueRetryStatements) insertQueueRetry(
	ctx context.Context,
	txn *sql.Tx,
	transactionID string,
	sendtype string,
	event gomatrixserverlib.Event,
	serverName gomatrixserverlib.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertRetryStmt)
	_, err := stmt.ExecContext(
		ctx,
		transactionID,          // the transaction ID that we initially attempted
		sendtype,               // either "pdu", "invite", "send_to_device"
		event.EventID(),        // the event ID
		event.OriginServerTS(), // the event origin server TS
		serverName,             // destination server name
		event.JSON(),           // JSON body
	)
	return err
}

func (s *queueRetryStatements) deleteQueueRetry(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteRetryStmt)
	_, err := stmt.ExecContext(ctx, pq.StringArray(eventIDs))
	return err
}

func (s *queueRetryStatements) selectRetryNextTransactionID(
	ctx context.Context, txn *sql.Tx, serverName, sendType string,
) (string, error) {
	var transactionID string
	stmt := sqlutil.TxStmt(txn, s.selectRetryNextTransactionIDStmt)
	err := stmt.QueryRowContext(ctx, serverName, types.FailedEventTypePDU).Scan(&transactionID)
	return transactionID, err
}

func (s *queueRetryStatements) selectQueueRetryPDUs(
	ctx context.Context, txn *sql.Tx, serverName string, transactionID string,
) ([]*gomatrixserverlib.HeaderedEvent, error) {

	stmt := sqlutil.TxStmt(txn, s.selectRetryPDUsByTransactionStmt)
	rows, err := stmt.QueryContext(ctx, serverName, types.FailedEventTypePDU, transactionID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "queueRetryFromStmt: rows.close() failed")

	var result []*gomatrixserverlib.HeaderedEvent
	for rows.Next() {
		var transactionID, eventID string
		var originServerTS int64
		var jsonBody []byte
		if err = rows.Scan(&transactionID, &eventID, &originServerTS, &jsonBody); err != nil {
			return nil, err
		}
		var event gomatrixserverlib.HeaderedEvent
		if err = json.Unmarshal(jsonBody, &event); err != nil {
			return nil, fmt.Errorf("json.Unmarshal: %w", err)
		}
		if event.EventID() != eventID {
			return nil, fmt.Errorf("event ID %q doesn't match expected %q", event.EventID(), eventID)
		}
		result = append(result, &event)
	}

	return result, rows.Err()
}
