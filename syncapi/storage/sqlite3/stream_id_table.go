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
`

const increaseStreamIDStmt = "" +
	"UPDATE syncapi_stream_id SET stream_id = stream_id + 1 WHERE stream_name = $1"

const selectStreamIDStmt = "" +
	"SELECT stream_id FROM syncapi_stream_id WHERE stream_name = $1"

type streamIDStatements struct {
	db                   *sql.DB
	writer               *sqlutil.TransactionWriter
	increaseStreamIDStmt *sql.Stmt
	selectStreamIDStmt   *sql.Stmt
}

func (s *streamIDStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	s.writer = sqlutil.NewTransactionWriter()
	_, err = db.Exec(streamIDTableSchema)
	if err != nil {
		return
	}
	if s.increaseStreamIDStmt, err = db.Prepare(increaseStreamIDStmt); err != nil {
		return
	}
	if s.selectStreamIDStmt, err = db.Prepare(selectStreamIDStmt); err != nil {
		return
	}
	return
}

func (s *streamIDStatements) nextStreamID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	selectStmt := sqlutil.TxStmt(txn, s.selectStreamIDStmt)
	err = s.writer.Do(s.db, nil, func(txn *sql.Tx) error {
		if _, ierr := increaseStmt.ExecContext(ctx, "global"); err != nil {
			return ierr
		}
		if serr := selectStmt.QueryRowContext(ctx, "global").Scan(&pos); err != nil {
			return serr
		}
		return nil
	})
	return
}
