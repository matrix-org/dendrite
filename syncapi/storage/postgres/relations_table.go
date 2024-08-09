// Copyright 2022 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

const relationsSchema = `
CREATE SEQUENCE IF NOT EXISTS syncapi_relation_id;

CREATE TABLE IF NOT EXISTS syncapi_relations (
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_relation_id'),
	room_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
	child_event_id TEXT NOT NULL,
	child_event_type TEXT NOT NULL,
	rel_type TEXT NOT NULL,
	CONSTRAINT syncapi_relations_unique UNIQUE (room_id, event_id, child_event_id, rel_type)
);
`

const insertRelationSQL = "" +
	"INSERT INTO syncapi_relations (" +
	"  room_id, event_id, child_event_id, child_event_type, rel_type" +
	") VALUES ($1, $2, $3, $4, $5) " +
	" ON CONFLICT DO NOTHING"

const deleteRelationSQL = "" +
	"DELETE FROM syncapi_relations WHERE room_id = $1 AND child_event_id = $2"

const selectRelationsInRangeAscSQL = "" +
	"SELECT id, child_event_id, rel_type FROM syncapi_relations" +
	" WHERE room_id = $1 AND event_id = $2" +
	" AND ( $3 = '' OR rel_type = $3 )" +
	" AND ( $4 = '' OR child_event_type = $4 )" +
	" AND id > $5 AND id <= $6" +
	" ORDER BY id ASC LIMIT $7"

const selectRelationsInRangeDescSQL = "" +
	"SELECT id, child_event_id, rel_type FROM syncapi_relations" +
	" WHERE room_id = $1 AND event_id = $2" +
	" AND ( $3 = '' OR rel_type = $3 )" +
	" AND ( $4 = '' OR child_event_type = $4 )" +
	" AND id >= $5 AND id < $6" +
	" ORDER BY id DESC LIMIT $7"

const selectThreadsSQL = "" +
	"SELECT syncapi_relations.id, syncapi_relations.event_id FROM syncapi_relations" +
	" JOIN syncapi_output_room_events ON syncapi_output_room_events.event_id = syncapi_relations.event_id" +
	" WHERE syncapi_relations.room_id = $1" +
	" AND syncapi_relations.rel_type = 'm.thread'" +
	" AND syncapi_relations.id >= $2" +
	" ORDER BY syncapi_relations.id LIMIT $3"

const selectThreadsWithSenderSQL = "" +
	"SELECT syncapi_relations.id, syncapi_relations.event_id FROM syncapi_relations" +
	" JOIN syncapi_output_room_events ON syncapi_output_room_events.event_id = syncapi_relations.event_id" +
	" WHERE syncapi_relations.room_id = $1" +
	" AND syncapi_output_room_events.sender = $2" +
	" AND syncapi_relations.rel_type = 'm.thread'" +
	" AND syncapi_relations.id >= $3" +
	" ORDER BY syncapi_relations.id LIMIT $4"

const selectMaxRelationIDSQL = "" +
	"SELECT COALESCE(MAX(id), 0) FROM syncapi_relations"

type relationsStatements struct {
	insertRelationStmt             *sql.Stmt
	selectRelationsInRangeAscStmt  *sql.Stmt
	selectRelationsInRangeDescStmt *sql.Stmt
	selectThreadsStmt              *sql.Stmt
	selectThreadsWithSenderStmt    *sql.Stmt
	deleteRelationStmt             *sql.Stmt
	selectMaxRelationIDStmt        *sql.Stmt
}

func NewPostgresRelationsTable(db *sql.DB) (tables.Relations, error) {
	s := &relationsStatements{}
	_, err := db.Exec(relationsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertRelationStmt, insertRelationSQL},
		{&s.selectRelationsInRangeAscStmt, selectRelationsInRangeAscSQL},
		{&s.selectRelationsInRangeDescStmt, selectRelationsInRangeDescSQL},
		{&s.selectThreadsStmt, selectThreadsSQL},
		{&s.selectThreadsWithSenderStmt, selectThreadsWithSenderSQL},
		{&s.deleteRelationStmt, deleteRelationSQL},
		{&s.selectMaxRelationIDStmt, selectMaxRelationIDSQL},
	}.Prepare(db)
}

func (s *relationsStatements) InsertRelation(
	ctx context.Context, txn *sql.Tx, roomID, eventID, childEventID, childEventType, relType string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.insertRelationStmt).ExecContext(
		ctx, roomID, eventID, childEventID, childEventType, relType,
	)
	return
}

func (s *relationsStatements) DeleteRelation(
	ctx context.Context, txn *sql.Tx, roomID, childEventID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteRelationStmt)
	_, err := stmt.ExecContext(
		ctx, roomID, childEventID,
	)
	return err
}

// SelectRelationsInRange returns a map rel_type -> []child_event_id
func (s *relationsStatements) SelectRelationsInRange(
	ctx context.Context, txn *sql.Tx, roomID, eventID, relType, eventType string,
	r types.Range, limit int,
) (map[string][]types.RelationEntry, types.StreamPosition, error) {
	var lastPos types.StreamPosition
	var stmt *sql.Stmt
	if r.Backwards {
		stmt = sqlutil.TxStmt(txn, s.selectRelationsInRangeDescStmt)
	} else {
		stmt = sqlutil.TxStmt(txn, s.selectRelationsInRangeAscStmt)
	}
	rows, err := stmt.QueryContext(ctx, roomID, eventID, relType, eventType, r.Low(), r.High(), limit)
	if err != nil {
		return nil, lastPos, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRelationsInRange: rows.close() failed")
	result := map[string][]types.RelationEntry{}
	var (
		id           types.StreamPosition
		childEventID string
		relationType string
	)
	for rows.Next() {
		if err = rows.Scan(&id, &childEventID, &relationType); err != nil {
			return nil, lastPos, err
		}
		if id > lastPos {
			lastPos = id
		}
		result[relationType] = append(result[relationType], types.RelationEntry{
			Position: id,
			EventID:  childEventID,
		})
	}
	if lastPos == 0 {
		lastPos = r.To
	}
	return result, lastPos, rows.Err()
}

func (s *relationsStatements) SelectThreads(
	ctx context.Context,
	txn *sql.Tx,
	roomID, userID string,
	from types.StreamPosition,
	limit uint64,
) ([]string, types.StreamPosition, error) {
	var lastPos types.StreamPosition
	var stmt *sql.Stmt
	var rows *sql.Rows
	var err error

	if userID == "" {
		stmt = sqlutil.TxStmt(txn, s.selectThreadsStmt)
		rows, err = stmt.QueryContext(ctx, roomID, from, limit)
	} else {
		stmt = sqlutil.TxStmt(txn, s.selectThreadsWithSenderStmt)
		rows, err = stmt.QueryContext(ctx, roomID, userID, from, limit)
	}
	if err != nil {
		return nil, lastPos, err
	}

	defer internal.CloseAndLogIfError(ctx, rows, "selectThreads: rows.close() failed")
	var result []string
	var (
		id      types.StreamPosition
		eventId string
	)

	for rows.Next() {
		if err = rows.Scan(&id, &eventId); err != nil {
			return nil, lastPos, err
		}
		if id > lastPos {
			lastPos = id
		}
		result = append(result, eventId)
	}

	return result, lastPos, rows.Err()
}

func (s *relationsStatements) SelectMaxRelationID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectMaxRelationIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&id)
	return
}
