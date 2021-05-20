package cosmosdb

import (
	"context"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
)

func GetNextRoomNID(s *roomStatements, ctx context.Context) (int64, error) {
	const docId = "roomnid_seq"
	return cosmosdbutil.GetNextSequence(ctx, s.db.connection, s.db.cosmosConfig, s.db.databaseName, s.tableName, docId, 1)
}
