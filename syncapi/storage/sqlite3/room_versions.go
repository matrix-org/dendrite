package sqlite3

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
)

type roomVersionStatements struct {
	selectStateEventStmt *sql.Stmt
}

func (s *roomVersionStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(currentRoomStateSchema)
	if err != nil {
		return
	}
	if s.selectStateEventStmt, err = db.Prepare(selectStateEventSQL); err != nil {
		return
	}
	return
}

func (s *roomVersionStatements) selectRoomVersion(
	ctx context.Context, txn *sql.Tx, roomID string,
) (roomVersion gomatrixserverlib.RoomVersion, err error) {
	stmt := common.TxStmt(txn, s.selectStateEventStmt)
	var res []byte
	err = stmt.QueryRowContext(ctx, roomID, "m.room.create", "").Scan(&res)
	if err != nil {
		return
	}
	rv := gjson.Get(string(res), "content.room_version")
	if !rv.Exists() {
		roomVersion = gomatrixserverlib.RoomVersionV1
		return
	}
	roomVersion = gomatrixserverlib.RoomVersion(rv.String())
	fmt.Println("room version for", roomID, "is", rv.String())
	return
}
