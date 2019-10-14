package storage

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/types"
)

// we treat send to device as abbrev as STD in the context below.

const sendToDeviceSchema = `
CREATE TABLE IF NOT EXISTS syncapi_send_to_device (
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_stream_id'),
	txn_id TEXT NOT NULL,
	sender TEXT NOT NULL,
	event_type TEXT NOT NULL,
	target_device_id TEXT NOT NULL,
	target_user_id TEXT NOT NULL,
	event_json TEXT NOT NULL,
	del_read INTEGER DEFAULT 0,
	max_read BIGINT DEFAULT currval('syncapi_stream_id')	,
CONSTRAINT syncapi_send_to_device_unique UNIQUE (txn_id, target_device_id, target_user_id)
);
`

const insertSTDSQL = "" +
	"INSERT INTO syncapi_send_to_device (" +
	" sender, event_type, target_user_id, target_device_id, txn_id, event_json" +
	") VALUES ($1, $2, $3, $4, $5, $6) RETURNING id"

const deleteSTDSQL = "" +
	"DELETE FROM syncapi_send_to_device WHERE target_user_id = $1 AND target_device_id = $2 AND max_read < $3 AND del_read = 1"

const selectSTDEventsInRangeSQL = "" +
	"SELECT id, sender, event_type, event_json FROM syncapi_send_to_device" +
	" WHERE target_user_id = $1 AND target_device_id = $2 AND id <= $3" +
	" ORDER BY id LIMIT 100 "

const updateSTDEventSQL = "" +
	"UPDATE syncapi_send_to_device SET del_read = 1 , max_read = $1 WHERE id = ANY($2)"

const selectMaxSTDIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_send_to_device"

type stdEventsStatements struct {
	insertStdEventStmt         *sql.Stmt
	selectStdEventsInRangeStmt *sql.Stmt
	deleteStdEventStmt         *sql.Stmt
	selectStdIDStmt            *sql.Stmt
	updateStdStmt              *sql.Stmt
}

func (s *stdEventsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(sendToDeviceSchema)
	if err != nil {
		return
	}
	if s.insertStdEventStmt, err = db.Prepare(insertSTDSQL); err != nil {
		return
	}
	if s.selectStdEventsInRangeStmt, err = db.Prepare(selectSTDEventsInRangeSQL); err != nil {
		return
	}
	if s.deleteStdEventStmt, err = db.Prepare(deleteSTDSQL); err != nil {
		return
	}
	if s.selectStdIDStmt, err = db.Prepare(selectMaxSTDIDSQL); err != nil {
		return
	}
	if s.updateStdStmt, err = db.Prepare(updateSTDEventSQL); err != nil {
		return
	}
	return
}

func (s *stdEventsStatements) insertStdEvent(
	ctx context.Context, stdEvent types.StdHolder,
	transactionID string, targetUID, targetDevice string,
) (streamPos int64, err error) {
	err = s.insertStdEventStmt.QueryRowContext(
		ctx,
		stdEvent.Sender,
		stdEvent.EventTyp,
		targetUID,
		targetDevice,
		transactionID,
		stdEvent.Event,
	).Scan(&streamPos)
	return
}

func (s *stdEventsStatements) deleteStdEvent(
	ctx context.Context, userID, deviceID string,
	idUpBound int64,
) error {
	_, err := s.deleteStdEventStmt.ExecContext(ctx, userID, deviceID, idUpBound)
	return err
}

func (s *stdEventsStatements) selectStdEventsInRange(
	ctx context.Context, txn *sql.Tx,
	targetUserID, targetDeviceID string,
	endPos int64,
) ([]types.StdHolder, error) {
	stdHolder := []types.StdHolder{}
	stmt := common.TxStmt(txn, s.selectStdEventsInRangeStmt)
	rows, err := stmt.QueryContext(ctx, targetUserID, targetDeviceID, endPos)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		holder := types.StdHolder{}
		var (
			id        int64
			sender    string
			eventType string
			eventJSON []byte
		)
		if err = rows.Scan(&id, &sender, &eventType, &eventJSON); err != nil {
			closeErr := rows.Close()
			if closeErr != nil {
				return nil, closeErr
			}
			return nil, err
		}
		holder.StreamID = id
		holder.Sender = sender
		holder.Event = eventJSON
		holder.EventTyp = eventType
		stdHolder = append(stdHolder, holder)
	}
	err = rows.Close()
	if err != nil {
		return nil, err
	}
	// update events with read mark
	update := []int64{}
	for _, val := range stdHolder {
		update = append(update, val.StreamID)
	}
	updateStmt := common.TxStmt(txn, s.updateStdStmt)
	_, err = updateStmt.ExecContext(ctx, endPos, pq.Array(update))
	if err != nil {
		return nil, err
	}
	return stdHolder, nil
}

func (s *stdEventsStatements) selectMaxStdID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := common.TxStmt(txn, s.selectStdIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
