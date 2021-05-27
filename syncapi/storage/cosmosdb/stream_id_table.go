package cosmosdb

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/syncapi/types"
)

// const streamIDTableSchema = `
// -- Global stream ID counter, used by other tables.
// CREATE TABLE IF NOT EXISTS syncapi_stream_id (
//   stream_name TEXT NOT NULL PRIMARY KEY,
//   stream_id INT DEFAULT 0,

//   UNIQUE(stream_name)
// );
// INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("global", 0)
//   ON CONFLICT DO NOTHING;
// INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("receipt", 0)
//   ON CONFLICT DO NOTHING;
// INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("accountdata", 0)
//   ON CONFLICT DO NOTHING;
// INSERT INTO syncapi_stream_id (stream_name, stream_id) VALUES ("invite", 0)
//   ON CONFLICT DO NOTHING;
// `

// const increaseStreamIDStmt = "" +
// 	"UPDATE syncapi_stream_id SET stream_id = stream_id + 1 WHERE stream_name = $1"

// const selectStreamIDStmt = "" +
// 	"SELECT stream_id FROM syncapi_stream_id WHERE stream_name = $1"

type streamIDStatements struct {
	db *SyncServerDatasource
	// increaseStreamIDStmt *sql.Stmt
	// selectStreamIDStmt   *sql.Stmt
	tableName string
}

func (s *streamIDStatements) prepare(db *SyncServerDatasource) (err error) {
	s.db = db
	s.tableName = "stream_id"
	return
}

func (s *streamIDStatements) nextPDUID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	const docId = "global_seq"
	result, err := cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 1)
	// increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	// selectStmt := sqlutil.TxStmt(txn, s.selectStreamIDStmt)
	// if _, err = increaseStmt.ExecContext(ctx, "global"); err != nil {
	// 	return
	// }
	// err = selectStmt.QueryRowContext(ctx, "global").Scan(&pos)
	if err != nil {
		return -1, err
	}
	pos = types.StreamPosition(result)
	return
}

func (s *streamIDStatements) nextReceiptID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	const docId = "receipt_seq"
	result, err := cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 1)
	// increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	// selectStmt := sqlutil.TxStmt(txn, s.selectStreamIDStmt)
	// if _, err = increaseStmt.ExecContext(ctx, "receipt"); err != nil {
	// 	return
	// }
	// err = selectStmt.QueryRowContext(ctx, "receipt").Scan(&pos)
	if err != nil {
		return -1, err
	}
	pos = types.StreamPosition(result)
	return
}

func (s *streamIDStatements) nextInviteID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	const docId = "invite_seq"
	result, err := cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 1)
	// increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	// selectStmt := sqlutil.TxStmt(txn, s.selectStreamIDStmt)
	// if _, err = increaseStmt.ExecContext(ctx, "invite"); err != nil {
	// 	return
	// }
	// err = selectStmt.QueryRowContext(ctx, "invite").Scan(&pos)
	if err != nil {
		return -1, err
	}
	pos = types.StreamPosition(result)
	return
}

func (s *streamIDStatements) nextAccountDataID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	const docId = "accountdata_seq"
	result, err := cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 1)
	// increaseStmt := sqlutil.TxStmt(txn, s.increaseStreamIDStmt)
	// selectStmt := sqlutil.TxStmt(txn, s.selectStreamIDStmt)
	// if _, err = increaseStmt.ExecContext(ctx, "accountdata"); err != nil {
	// 	return
	// }
	// err = selectStmt.QueryRowContext(ctx, "accountdata").Scan(&pos)
	if err != nil {
		return -1, err
	}
	pos = types.StreamPosition(result)
	return
}
