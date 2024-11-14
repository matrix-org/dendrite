// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const reportedEventsScheme = `
CREATE SEQUENCE IF NOT EXISTS roomserver_reported_events_id_seq;
CREATE TABLE IF NOT EXISTS roomserver_reported_events
(
	id 					BIGINT PRIMARY KEY DEFAULT nextval('roomserver_reported_events_id_seq'),
    room_nid 			BIGINT NOT NULL,
	event_nid 			BIGINT NOT NULL,
    reporting_user_nid	BIGINT NOT NULL, -- the user reporting the event
    event_sender_nid	BIGINT NOT NULL, -- the user who sent the reported event
    reason      		TEXT,
    score       		INTEGER,
    received_ts 		BIGINT NOT NULL
);`

const insertReportedEventSQL = `
	INSERT INTO roomserver_reported_events (room_nid, event_nid, reporting_user_nid, event_sender_nid, reason, score, received_ts) 
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	RETURNING id
`

const selectReportedEventsDescSQL = `
WITH countReports AS (
    SELECT count(*) as report_count
    FROM roomserver_reported_events
    WHERE ($1::BIGINT IS NULL OR room_nid = $1::BIGINT) AND ($2::TEXT IS NULL OR reporting_user_nid = $2::BIGINT)
)
SELECT report_count, id, room_nid, event_nid, reporting_user_nid, event_sender_nid, reason, score, received_ts
FROM roomserver_reported_events, countReports
WHERE ($1::BIGINT IS NULL OR room_nid = $1::BIGINT) AND ($2::TEXT IS NULL OR reporting_user_nid = $2::BIGINT)
ORDER BY received_ts DESC
OFFSET $3
LIMIT $4
`

const selectReportedEventsAscSQL = `
WITH countReports AS (
    SELECT count(*) as report_count
    FROM roomserver_reported_events
    WHERE ($1::BIGINT IS NULL OR room_nid = $1::BIGINT) AND ($2::TEXT IS NULL OR reporting_user_nid = $2::BIGINT)
)
SELECT report_count, id, room_nid, event_nid, reporting_user_nid, event_sender_nid, reason, score, received_ts
FROM roomserver_reported_events, countReports
WHERE ($1::BIGINT IS NULL OR room_nid = $1::BIGINT) AND ($2::TEXT IS NULL OR reporting_user_nid = $2::BIGINT)
ORDER BY received_ts ASC
OFFSET $3
LIMIT $4
`

const selectReportedEventSQL = `
SELECT id, room_nid, event_nid, reporting_user_nid, event_sender_nid, reason, score, received_ts
FROM roomserver_reported_events
WHERE id = $1
`

const deleteReportedEventSQL = `DELETE FROM roomserver_reported_events WHERE id = $1`

type reportedEventsStatements struct {
	insertReportedEventsStmt     *sql.Stmt
	selectReportedEventsDescStmt *sql.Stmt
	selectReportedEventsAscStmt  *sql.Stmt
	selectReportedEventStmt      *sql.Stmt
	deleteReportedEventStmt      *sql.Stmt
}

func CreateReportedEventsTable(db *sql.DB) error {
	_, err := db.Exec(reportedEventsScheme)
	return err
}

func PrepareReportedEventsTable(db *sql.DB) (tables.ReportedEvents, error) {
	s := &reportedEventsStatements{}

	return s, sqlutil.StatementList{
		{&s.insertReportedEventsStmt, insertReportedEventSQL},
		{&s.selectReportedEventsDescStmt, selectReportedEventsDescSQL},
		{&s.selectReportedEventsAscStmt, selectReportedEventsAscSQL},
		{&s.selectReportedEventStmt, selectReportedEventSQL},
		{&s.deleteReportedEventStmt, deleteReportedEventSQL},
	}.Prepare(db)
}

func (r *reportedEventsStatements) InsertReportedEvent(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventNID types.EventNID,
	reportingUserID types.EventStateKeyNID,
	eventSenderID types.EventStateKeyNID,
	reason string,
	score int64,
) (int64, error) {
	stmt := sqlutil.TxStmt(txn, r.insertReportedEventsStmt)

	var reportID int64
	err := stmt.QueryRowContext(ctx,
		roomNID,
		eventNID,
		reportingUserID,
		eventSenderID,
		reason,
		score,
		spec.AsTimestamp(time.Now()),
	).Scan(&reportID)
	return reportID, err
}

func (r *reportedEventsStatements) SelectReportedEvents(
	ctx context.Context,
	txn *sql.Tx,
	from, limit uint64,
	backwards bool,
	reportingUserID types.EventStateKeyNID,
	roomNID types.RoomNID,
) ([]api.QueryAdminEventReportsResponse, int64, error) {
	var stmt *sql.Stmt
	if backwards {
		stmt = sqlutil.TxStmt(txn, r.selectReportedEventsDescStmt)
	} else {
		stmt = sqlutil.TxStmt(txn, r.selectReportedEventsAscStmt)
	}

	var qryRoomNID *types.RoomNID
	if roomNID > 0 {
		qryRoomNID = &roomNID
	}
	var qryReportingUser *types.EventStateKeyNID
	if reportingUserID > 0 {
		qryReportingUser = &reportingUserID
	}

	rows, err := stmt.QueryContext(ctx,
		qryRoomNID,
		qryReportingUser,
		from,
		limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectReportedEvents: failed to close rows")

	var result []api.QueryAdminEventReportsResponse
	var row api.QueryAdminEventReportsResponse
	var count int64
	for rows.Next() {
		if err = rows.Scan(
			&count,
			&row.ID,
			&row.RoomNID,
			&row.EventNID,
			&row.ReportingUserNID,
			&row.SenderNID,
			&row.Reason,
			&row.Score,
			&row.ReceivedTS,
		); err != nil {
			return nil, 0, err
		}
		result = append(result, row)
	}

	return result, count, rows.Err()
}

func (r *reportedEventsStatements) SelectReportedEvent(
	ctx context.Context,
	txn *sql.Tx,
	reportID uint64,
) (api.QueryAdminEventReportResponse, error) {
	stmt := sqlutil.TxStmt(txn, r.selectReportedEventStmt)

	var row api.QueryAdminEventReportResponse
	if err := stmt.QueryRowContext(ctx, reportID).Scan(
		&row.ID,
		&row.RoomNID,
		&row.EventNID,
		&row.ReportingUserNID,
		&row.SenderNID,
		&row.Reason,
		&row.Score,
		&row.ReceivedTS,
	); err != nil {
		return api.QueryAdminEventReportResponse{}, err
	}
	return row, nil
}

func (r *reportedEventsStatements) DeleteReportedEvent(ctx context.Context, txn *sql.Tx, reportID uint64) error {
	stmt := sqlutil.TxStmt(txn, r.deleteReportedEventStmt)
	_, err := stmt.ExecContext(ctx, reportID)
	return err
}
