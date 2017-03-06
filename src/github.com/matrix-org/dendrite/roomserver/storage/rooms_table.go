package storage

import (
	"database/sql"
	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const roomsSchema = `
CREATE SEQUENCE IF NOT EXISTS room_nid_seq;
CREATE TABLE IF NOT EXISTS rooms (
    -- Local numeric ID for the room.
    room_nid BIGINT PRIMARY KEY DEFAULT nextval('room_nid_seq'),
    -- Textual ID for the room.
    room_id TEXT NOT NULL CONSTRAINT room_id_unique UNIQUE,
    -- The most recent events in the room that aren't referenced by another event.
    -- This list may empty if the server hasn't joined the room yet.
    -- (The server will be in that state while it stores the events for the initial state of the room)
    latest_event_nids BIGINT[] NOT NULL DEFAULT '{}'::BIGINT[],
    -- The last event written to the output log for this room.
    last_event_sent_nid BIGINT NOT NULL DEFAULT 0
);
`

// Same as insertEventTypeNIDSQL
const insertRoomNIDSQL = "" +
	"INSERT INTO rooms (room_id) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT room_id_unique" +
	" DO NOTHING RETURNING (room_nid)"

const selectRoomNIDSQL = "" +
	"SELECT room_nid FROM rooms WHERE room_id = $1"

const selectLatestEventNIDsSQL = "" +
	"SELECT latest_event_nids FROM rooms WHERE room_nid = $1"

const selectLatestEventNIDsForUpdateSQL = "" +
	"SELECT latest_event_nids, last_event_sent_nid FROM rooms WHERE room_nid = $1 FOR UPDATE"

const updateLatestEventNIDsSQL = "" +
	"UPDATE rooms SET latest_event_nids = $2, last_event_sent_nid = $3 WHERE room_nid = $1"

type roomStatements struct {
	insertRoomNIDStmt                  *sql.Stmt
	selectRoomNIDStmt                  *sql.Stmt
	selectLatestEventNIDsStmt          *sql.Stmt
	selectLatestEventNIDsForUpdateStmt *sql.Stmt
	updateLatestEventNIDsStmt          *sql.Stmt
}

func (s *roomStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(roomsSchema)
	if err != nil {
		return
	}
	if s.insertRoomNIDStmt, err = db.Prepare(insertRoomNIDSQL); err != nil {
		return
	}
	if s.selectRoomNIDStmt, err = db.Prepare(selectRoomNIDSQL); err != nil {
		return
	}
	if s.selectLatestEventNIDsStmt, err = db.Prepare(selectLatestEventNIDsSQL); err != nil {
		return
	}
	if s.selectLatestEventNIDsForUpdateStmt, err = db.Prepare(selectLatestEventNIDsForUpdateSQL); err != nil {
		return
	}
	if s.updateLatestEventNIDsStmt, err = db.Prepare(updateLatestEventNIDsSQL); err != nil {
		return
	}
	return
}

func (s *roomStatements) insertRoomNID(roomID string) (types.RoomNID, error) {
	var roomNID int64
	err := s.insertRoomNIDStmt.QueryRow(roomID).Scan(&roomNID)
	return types.RoomNID(roomNID), err
}

func (s *roomStatements) selectRoomNID(roomID string) (types.RoomNID, error) {
	var roomNID int64
	err := s.selectRoomNIDStmt.QueryRow(roomID).Scan(&roomNID)
	return types.RoomNID(roomNID), err
}

func (s *roomStatements) selectLatestEventNIDs(roomNID types.RoomNID) ([]types.EventNID, error) {
	var nids pq.Int64Array
	err := s.selectLatestEventNIDsStmt.QueryRow(int64(roomNID)).Scan(&nids)
	if err != nil {
		return nil, err
	}
	eventNIDs := make([]types.EventNID, len(nids))
	for i := range nids {
		eventNIDs[i] = types.EventNID(nids[i])
	}
	return eventNIDs, nil
}

func (s *roomStatements) selectLatestEventsNIDsForUpdate(txn *sql.Tx, roomNID types.RoomNID) ([]types.EventNID, types.EventNID, error) {
	var nids pq.Int64Array
	var lastEventSentNID int64
	err := txn.Stmt(s.selectLatestEventNIDsForUpdateStmt).QueryRow(int64(roomNID)).Scan(&nids, &lastEventSentNID)
	if err != nil {
		return nil, 0, err
	}
	eventNIDs := make([]types.EventNID, len(nids))
	for i := range nids {
		eventNIDs[i] = types.EventNID(nids[i])
	}
	return eventNIDs, types.EventNID(lastEventSentNID), nil
}

func (s *roomStatements) updateLatestEventNIDs(txn *sql.Tx, roomNID types.RoomNID, eventNIDs []types.EventNID, lastEventSentNID types.EventNID) error {
	_, err := txn.Stmt(s.updateLatestEventNIDsStmt).Exec(roomNID, eventNIDsAsArray(eventNIDs), int64(lastEventSentNID))
	return err
}
