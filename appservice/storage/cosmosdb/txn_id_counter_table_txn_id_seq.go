package cosmosdb

import (
	"context"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
)

func GetNextCounterTXNID(s *txnStatements, ctx context.Context) (int, error) {
	const docId = "txn_id_seq"
	result, err := cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 1)
	if(err != nil) {
		return -1, err
	}
	return int(result), err
}
