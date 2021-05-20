package cosmosdb

import (
	"context"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
)

func GetNextEventStateKeyNID(s *eventStateKeyStatements, ctx context.Context) (int64, error) {
	const docId = "eventstatekeynid_seq"
	//1 insert start at 2
	return cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 2)
}

func GetNextEventTypeNID(s *eventTypeStatements, ctx context.Context) (int64, error) {
	const docId = "eventtypenid_seq"
	//7 inserts start at 8
	return cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 8)
}

func GetNextEventNID(s *eventStatements, ctx context.Context) (int64, error) {
	const docId = "eventnid_seq"
	return cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 1)
}
