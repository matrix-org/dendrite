package cosmosdbapi

import (
	"context"
	"fmt"
)

func GetDocumentId(tenantName string, collectionName string, id string) string {
	return fmt.Sprintf("%s,%s,%s", collectionName, tenantName, id)
}

func GetPartitionKey(tenantName string, collectionName string) string {
	return fmt.Sprintf("%s,%s", collectionName, tenantName)
}

func GetDocumentOrNil(connection CosmosConnection, config CosmosConfig, ctx context.Context, partitionKey string, cosmosDocId string, dbData interface{}) error {
	var _, err = GetClient(connection).GetDocument(
		ctx,
		config.DatabaseName,
		config.ContainerName,
		cosmosDocId,
		GetGetDocumentOptions(partitionKey),
		&dbData,
	)

	if err != nil {
		if err.Error() == "Resource that no longer exists" {
			dbData = nil
			return nil
		}
		return err
	}

	return nil
}
