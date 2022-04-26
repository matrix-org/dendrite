package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/types"
)

const streamIDTableSchema = `
-- Global stream ID counter, used by other tables.
CREATE TABLE IF NOT EXISTS syncapi_stream_id (
  stream_name TEXT NOT NULL PRIMARY KEY,
  stream_id INT DEFAULT 0,

  UNIQUE(stream_name)
);
INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("global", 0)
  ON CONFLICT DO NOTHING;
INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("receipt", 0)
  ON CONFLICT DO NOTHING;
INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("accountdata", 0)
  ON CONFLICT DO NOTHING;
INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("invite", 0)
  ON CONFLICT DO NOTHING;
INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("presence", 0)
  ON CONFLICT DO NOTHING;
`

const increaseStreamIDStmt = "" +
	"UPDATE syncapi_stream_id SET stream_id = stream_id + 1 WHERE stream_name = $1" +
	" RETURNING stream_id"

type StreamIDStatements struct {
	db                   *sql.DB
	increaseStreamIDStmt *sql.Stmt
}

func (s *StreamIDStatements) Prepare(db *sql.DB) (err error) {
	s.db = db
	_, err = db.Exec(streamIDTableSchema)
	if err != nil {
		return
	}
	if s.increaseStreamIDStmt, err = db.Prepare(increaseStreamIDStmt); err != nil {
		return
	}
	return
}

func (s *StreamIDStatements) nextPDUID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	err = increaseStmt.QueryRowContext(ctx, "global").Scan(&pos)
	return
}

func (s *StreamIDStatements) nextReceiptID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	err = increaseStmt.QueryRowContext(ctx, "receipt").Scan(&pos)
	return
}

func (s *StreamIDStatements) nextInviteID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	err = increaseStmt.QueryRowContext(ctx, "invite").Scan(&pos)
	return
}

func (s *StreamIDStatements) nextAccountDataID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	err = increaseStmt.QueryRowContext(ctx, "accountdata").Scan(&pos)
	return
}

func (s *StreamIDStatements) nextPresenceID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	err = increaseStmt.QueryRowContext(ctx, "presence").Scan(&pos)
	return
}
