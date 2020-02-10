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

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationsender/types"
)

const retrySchema = `
CREATE TABLE IF NOT EXISTS federationsender_retry (
  retry_nid BIGSERIAL PRIMARY KEY,
  origin_server_name TEXT NOT NULL,
  destination_server_name TEXT NOT NULL,
  event_json BYTEA NOT NULL,
  attempts BIGINT DEFAULT 0,
  retry_at BIGINT NOT NULL,
  CONSTRAINT federationsender_retry_unique UNIQUE (origin_server_name, destination_server_name, event_json)
);`

const upsertEventSQL = `
INSERT INTO federationsender_retry
	(origin_server_name, destination_server_name, event_json, attempts, retry_at)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT ON CONSTRAINT federationsender_retry_unique
  DO UPDATE SET
    attempts = federationsender_retry.attempts+1,
    retry_at = $5
`

const deleteEventSQL = `
	DELETE FROM federationsender_retry WHERE retry_nid = $1
`

const selectEventsForRetry = `
  SELECT * FROM federationsender_retry WHERE retry_at >= $1 AND attempts < 5
    ORDER BY retry_at
`

const deleteExpiredEvents = `
	DELETE FROM federationsender_retry WHERE attempts >= 5 OR retry_at < $1
`

type retryStatements struct {
	upsertEventStmt          *sql.Stmt
	deleteEventStmt          *sql.Stmt
	selectEventsForRetryStmt *sql.Stmt
	deleteExpiredEventsStmt  *sql.Stmt
}

func (s *retryStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(retrySchema)
	if err != nil {
		return
	}

	if s.upsertEventStmt, err = db.Prepare(upsertEventSQL); err != nil {
		return
	}
	if s.deleteEventStmt, err = db.Prepare(deleteEventSQL); err != nil {
		return
	}
	if s.selectEventsForRetryStmt, err = db.Prepare(selectEventsForRetry); err != nil {
		return
	}
	if s.deleteExpiredEventsStmt, err = db.Prepare(deleteExpiredEvents); err != nil {
		return
	}
	return
}

func (s *retryStatements) upsertRetryEvent(
	ctx context.Context, txn *sql.Tx,
	originServer string, destinationServer string, eventJSON []byte,
	attempts int, retryAt int64,
) error {
	_, err := common.TxStmt(txn, s.upsertEventStmt).ExecContext(
		ctx, originServer, destinationServer,
		eventJSON, attempts, retryAt,
	)
	return err
}

func (s *retryStatements) deleteRetryEvent(
	ctx context.Context, txn *sql.Tx, retryNID int64,
) error {
	_, err := common.TxStmt(txn, s.deleteEventStmt).ExecContext(
		ctx, retryNID,
	)
	return err
}

func (s *retryStatements) selectRetryEventsPending(
	ctx context.Context, txn *sql.Tx,
) ([]*types.PendingPDU, error) {
	var pending []*types.PendingPDU
	stmt := common.TxStmt(txn, s.selectEventsForRetryStmt)
	rows, err := stmt.QueryContext(ctx, time.Now().UTC().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry types.PendingPDU
		var rawEvent []byte
		if err = rows.Scan(
			&entry.RetryNID, &entry.Origin, &entry.Destination,
			&rawEvent, &entry.Attempts, &entry.Attempts,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawEvent, &entry.PDU); err != nil {
			return nil, err
		}
		pending = append(pending, &entry)
	}
	return pending, err
}

func (s *retryStatements) deleteRetryExpiredEvents(
	ctx context.Context, txn *sql.Tx,
) error {
	stmt := common.TxStmt(txn, s.deleteExpiredEventsStmt)
	_, err := stmt.ExecContext(ctx, time.Now().UTC().Unix())
	return err
}
