package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

//go:embed schema.sql
var schema string

var selectMultiRoomCastSQL = `SELECT d.user_id, d.type, d.data, d.ts FROM syncapi_multiroom_data AS d
JOIN syncapi_multiroom_visibility AS v
ON d.user_id = v.user_id
AND d.type = v.type
WHERE v.room_id = ANY($1)
AND id > $2
AND id <= $3`

type multiRoomStatements struct {
	selectMultiRoomCast *sql.Stmt
}

func NewPostgresMultiRoomCastTable(db *sql.DB) (tables.MultiRoom, error) {
	r := &multiRoomStatements{}
	_, err := db.Exec(schema)
	if err != nil {
		return nil, err
	}
	return r, sqlutil.StatementList{
		{&r.selectMultiRoomCast, selectMultiRoomCastSQL},
	}.Prepare(db)
}

func (s *multiRoomStatements) SelectMultiRoomData(ctx context.Context, r *types.Range, joinedRooms []string, txn *sql.Tx) ([]*types.MultiRoomDataRow, error) {
	rows, err := sqlutil.TxStmt(txn, s.selectMultiRoomCast).QueryContext(ctx, pq.StringArray(joinedRooms), r.Low(), r.High())
	if err != nil {
		return nil, err
	}
	data := make([]*types.MultiRoomDataRow, 0)
	defer internal.CloseAndLogIfError(ctx, rows, "SelectMultiRoomData: rows.close() failed")
	var t time.Time
	for rows.Next() {
		r := types.MultiRoomDataRow{}
		err = rows.Scan(&r.UserId, &r.Type, &r.Data, &t)
		r.Timestamp = t.UnixMilli()
		if err != nil {
			return nil, fmt.Errorf("rows scan: %w", err)
		}
		data = append(data, &r)
	}
	return data, rows.Err()
}
