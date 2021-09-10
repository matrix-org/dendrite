package cosmosdb

import (
	"context"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
)

func GetNextStateBlockNID(s *stateBlockStatements, ctx context.Context) (int64, error) {
	const docId = "stateblocknid_seq"
	//1 insert start at 2
	return cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 2)
}
