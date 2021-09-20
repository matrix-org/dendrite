package cosmosdbutil

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
)

type SequenceCosmosData struct {
	cosmosdbapi.CosmosDocument
	Value int64 `json:"_value"`
}

func GetNextSequence(
	ctx context.Context,
	connection cosmosdbapi.CosmosConnection,
	config cosmosdbapi.CosmosConfig,
	serviceName string,
	tableName string,
	seqId string,
	initial int64,
) (int64, error) {
	collName := fmt.Sprintf("%s_%s", tableName, seqId)
	dbCollectionName := cosmosdbapi.GetCollectionName(serviceName, collName)
	cosmosDocId := cosmosdbapi.GetDocumentId(config.ContainerName, dbCollectionName, seqId)
	pk := cosmosDocId

	dbData := SequenceCosmosData{}
	cosmosdbapi.GetDocumentOrNil(
		connection,
		config,
		ctx,
		pk,
		cosmosDocId,
		&dbData,
	)

	if dbData.Id == "" {
		dbData = SequenceCosmosData{}
		dbData.CosmosDocument = cosmosdbapi.GenerateDocument(dbCollectionName, config.TenantName, pk, cosmosDocId)
		dbData.Value = initial
		var optionsCreate = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
		var _, _, err = cosmosdbapi.GetClient(connection).CreateDocument(
			ctx,
			config.DatabaseName,
			config.ContainerName,
			dbData,
			optionsCreate,
		)
		if err != nil {
			return -1, err
		}
	} else {
		dbData.Value++
		var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(dbData.Pk, dbData.ETag)
		var _, _, err = cosmosdbapi.GetClient(connection).ReplaceDocument(
			ctx,
			config.DatabaseName,
			config.ContainerName,
			cosmosDocId,
			dbData,
			optionsReplace,
		)
		if err != nil {
			return -1, err
		}
	}
	return dbData.Value, nil
}
