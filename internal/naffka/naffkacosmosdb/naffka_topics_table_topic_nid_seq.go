package naffkacosmosdb

import (
	"context"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
)

func GetNextTopicNID(s *topicsStatements, ctx context.Context) (int64, error) {
	const docId = "topic_nid_seq"
	return cosmosdbutil.GetNextSequence(ctx, s.DB.connection, s.DB.cosmosConfig, s.DB.databaseName, s.tableNameTopics, docId, 1)
}
