package cosmosdbutil

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
)

type SequenceCosmosData struct {
	Id        string `json:"id"`
	Pk        string `json:"_pk"`
	Tn        string `json:"_sid"`
	Cn        string `json:"_cn"`
	ETag      string `json:"_etag"`
	Timestamp int64  `json:"_ts"`
	Value     int64  `json:"_value"`
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
		dbData.Id = cosmosDocId
		dbData.Pk = pk
		dbData.Tn = config.TenantName
		dbData.Cn = dbCollectionName
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
