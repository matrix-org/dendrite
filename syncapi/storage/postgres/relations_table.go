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
	rel_type TEXT NOT NULL,
	CONSTRAINT syncapi_relations_unique UNIQUE (room_id, event_id, child_event_id, rel_type)
);
`

const insertRelationSQL = "" +
	"INSERT INTO syncapi_relations (" +
	"  room_id, event_id, child_event_id, rel_type" +
	") VALUES ($1, $2, $3, $4) " +
	" ON CONFLICT ON CONSTRAINT syncapi_relations_unique DO UPDATE SET event_id=EXCLUDED.event_id" +
	" RETURNING id"

const deleteRelationSQL = "" +
	"DELETE FROM syncapi_relations WHERE room_id = $1 AND child_event_id = $2"

const selectRelationsInRangeAscSQL = "" +
	"SELECT id, room_id, child_event_id, rel_type FROM syncapi_relations" +
	" WHERE room_id = $1 AND event_id = $2 AND id > $3 AND id <= $4" +
	" ORDER BY id ASC LIMIT $5"

const selectRelationsInRangeDescSQL = "" +
	"SELECT id, room_id, child_event_id, rel_type FROM syncapi_relations" +
	" WHERE room_id = $1 AND event_id = $2 AND id >= $3 AND id < $4" +
	" ORDER BY id DESC LIMIT $5"

const selectRelationsByTypeInRangeAscSQL = "" +
	"SELECT id, room_id, child_event_id, rel_type FROM syncapi_relations" +
	" WHERE room_id = $1 AND event_id = $2 AND rel_type = $3 AND id > $4 AND id <= $5" +
	" ORDER BY id ASC LIMIT $6"

const selectRelationsByTypeInRangeDescSQL = "" +
	"SELECT id, room_id, child_event_id, rel_type FROM syncapi_relations" +
	" WHERE room_id = $1 AND event_id = $2 AND rel_type = $3 AND id >= $4 AND id < $5" +
	" ORDER BY id DESC LIMIT $6"

const selectMaxRelationIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_relations"

type relationsStatements struct {
	insertRelationStmt                   *sql.Stmt
	selectRelationsInRangeAscStmt        *sql.Stmt
	selectRelationsInRangeDescStmt       *sql.Stmt
	selectRelationsByTypeInRangeAscStmt  *sql.Stmt
	selectRelationsByTypeInRangeDescStmt *sql.Stmt
	deleteRelationStmt                   *sql.Stmt
	selectMaxRelationIDStmt              *sql.Stmt
}

func NewPostgresRelationsTable(db *sql.DB) (tables.Relations, error) {
	s := &relationsStatements{}
	_, err := db.Exec(relationsSchema)
	if err != nil {
		return nil, err
	}
	if s.insertRelationStmt, err = db.Prepare(insertRelationSQL); err != nil {
		return nil, err
	}
	if s.selectRelationsInRangeAscStmt, err = db.Prepare(selectRelationsInRangeAscSQL); err != nil {
		return nil, err
	}
	if s.selectRelationsInRangeDescStmt, err = db.Prepare(selectRelationsInRangeDescSQL); err != nil {
		return nil, err
	}
	if s.selectRelationsByTypeInRangeAscStmt, err = db.Prepare(selectRelationsByTypeInRangeAscSQL); err != nil {
		return nil, err
	}
	if s.selectRelationsByTypeInRangeDescStmt, err = db.Prepare(selectRelationsByTypeInRangeDescSQL); err != nil {
		return nil, err
	}
	if s.deleteRelationStmt, err = db.Prepare(deleteRelationSQL); err != nil {
		return nil, err
	}
	if s.selectMaxRelationIDStmt, err = db.Prepare(selectMaxRelationIDSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *relationsStatements) InsertRelation(
	ctx context.Context, txn *sql.Tx, roomID, eventID, childEventID, relType string,
) (streamPos types.StreamPosition, err error) {
	err = sqlutil.TxStmt(txn, s.insertRelationStmt).QueryRowContext(
		ctx, roomID, eventID, childEventID, relType,
	).Scan(&streamPos)
	return
}

func (s *relationsStatements) DeleteRelation(
	ctx context.Context, txn *sql.Tx, roomID, childEventID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteRelationStmt)
	_, err := stmt.ExecContext(
		ctx, roomID, childEventID,
	)
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

// SelectRelationsInRange returns a map rel_type -> []child_event_id
func (s *relationsStatements) SelectRelationsInRange(
	ctx context.Context, txn *sql.Tx, roomID, eventID, relType string,
	r types.Range, limit int,
) (map[string][]types.RelationEntry, types.StreamPosition, error) {
	var lastPos types.StreamPosition
	var stmt *sql.Stmt
	var rows *sql.Rows
	var err error
	if relType != "" {
		if r.Backwards {
			stmt = sqlutil.TxStmt(txn, s.selectRelationsByTypeInRangeDescStmt)
		} else {
			stmt = sqlutil.TxStmt(txn, s.selectRelationsByTypeInRangeAscStmt)
		}
		rows, err = stmt.QueryContext(ctx, roomID, eventID, relType, r.Low(), r.High(), limit)
	} else {
		if r.Backwards {
			stmt = sqlutil.TxStmt(txn, s.selectRelationsInRangeDescStmt)
		} else {
			stmt = sqlutil.TxStmt(txn, s.selectRelationsInRangeAscStmt)
		}
		rows, err = stmt.QueryContext(ctx, roomID, eventID, r.Low(), r.High(), limit)
	}
	if err != nil {
		return nil, lastPos, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRelationsInRange: rows.close() failed")
	result := map[string][]types.RelationEntry{}
	for rows.Next() {
		var (
			id           types.StreamPosition
			roomID       string
			childEventID string
			relType      string
		)
		if err = rows.Scan(&id, &roomID, &childEventID, &relType); err != nil {
			return nil, lastPos, err
		}
		if id > lastPos {
			lastPos = id
		}
		result[relType] = append(result[relType], types.RelationEntry{
			Position: id,
			EventID:  childEventID,
		})
	}
	if lastPos == 0 {
		lastPos = r.To
	}
	return result, lastPos, rows.Err()
}

func (s *relationsStatements) SelectMaxRelationID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := sqlutil.TxStmt(txn, s.selectMaxRelationIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
