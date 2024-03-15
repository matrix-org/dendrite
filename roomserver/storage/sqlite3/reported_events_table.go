// Copyright 2023 The Matrix.org Foundation C.I.C.
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
	"time"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const reportedEventsScheme = `
CREATE SEQUENCE IF NOT EXISTS roomserver_reported_events_id_seq;
CREATE TABLE IF NOT EXISTS roomserver_reported_events
(
	id 			BIGINT PRIMARY KEY DEFAULT nextval('roomserver_reported_events_id_seq'),
    room_nid 	BIGINT NOT NULL,
	event_nid 	BIGINT NOT NULL,
    user_id     TEXT NOT NULL,
    reason      TEXT,
    score       INTEGER,
    received_ts BIGINT NOT NULL
);`

const insertReportedEventSQL = `
	INSERT INTO roomserver_reported_events (room_nid, event_nid, user_id, reason, score, received_ts) 
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id
`

type reportedEventsStatements struct {
	insertReportedEventsStmt *sql.Stmt
}

func CreateReportedEventsTable(db *sql.DB) error {
	_, err := db.Exec(reportedEventsScheme)
	return err
}

func PrepareReportEventsTable(db *sql.DB) (tables.ReportedEvents, error) {
	s := &reportedEventsStatements{}

	return s, sqlutil.StatementList{
		{&s.insertReportedEventsStmt, insertReportedEventSQL},
	}.Prepare(db)
}

func (r *reportedEventsStatements) InsertReportedEvent(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventNID types.EventNID,
	reportingUserID string,
	reason string,
	score int64,
) (int64, error) {
	stmt := sqlutil.TxStmt(txn, r.insertReportedEventsStmt)

	var reportID int64
	err := stmt.QueryRowContext(ctx,
		roomNID,
		eventNID,
		reportingUserID,
		reason,
		score,
		spec.AsTimestamp(time.Now()),
	).Scan(&reportID)
	return reportID, err
}
