package storage

import (
	"database/sql"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const roomsSchema = `
CREATE SEQUENCE IF NOT EXISTS room_nid_seq;
CREATE TABLE IF NOT EXISTS rooms (
    -- Local numeric ID for the room.
    room_nid BIGINT PRIMARY KEY DEFAULT nextval('room_nid_seq'),
    -- Textual ID for the room.
    room_id TEXT NOT NULL CONSTRAINT room_id_unique UNIQUE
);
`

// Same as insertEventTypeNIDSQL
const insertRoomNIDSQL = "" +
	"INSERT INTO rooms (room_id) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT room_id_unique" +
	" DO UPDATE SET room_id = $1" +
	" RETURNING (room_nid)"

const selectRoomNIDSQL = "" +
	"SELECT room_nid FROM rooms WHERE room_id = $1"

type roomStatements struct {
	insertRoomNIDStmt *sql.Stmt
	selectRoomNIDStmt *sql.Stmt
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
