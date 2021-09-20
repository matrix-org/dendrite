package cosmosdbapi

import (
	"context"
	"time"

	cosmosapi "github.com/vippsas/go-cosmosdb/cosmosapi"
)

type CosmosConnection struct {
	Url string
	Key string
}

func GetCosmosConnection(accountEndpoint string, accountKey string) CosmosConnection {
	return CosmosConnection{
		Url: accountEndpoint,
		Key: accountKey,
	}
}

func GetClient(conn CosmosConnection) *cosmosapi.Client {
	cfg := cosmosapi.Config{
		MasterKey: conn.Key,
	}
	return cosmosapi.New(conn.Url, cfg, nil, nil)
}

func UpsertDocument(ctx context.Context,
	conn CosmosConnection,
	databaseName string,
	containerName string,
	partitonKey string,
	dbData interface{}) error {
	var options = getUpsertDocumentOptions(partitonKey)
	_, _, err := GetClient(conn).CreateDocument(
		ctx,
		databaseName,
		containerName,
		&dbData,
		options)
	return err
}

func (doc *CosmosDocument) SetUpdateTime() {
	now := time.Now().UTC()
	doc.Ut = now.Format(time.RFC3339)
}

func GenerateDocument(
	collection string,
	tenantName string,
	partitionKey string,
	docId string,
) CosmosDocument {
	doc := CosmosDocument{}
	now := time.Now().UTC()
	doc.Timestamp = now.Unix()
	doc.Ct = now.Format(time.RFC3339)
	doc.Ut = now.Format(time.RFC3339)
	doc.Cn = collection
	doc.Tn = tenantName
	doc.Pk = partitionKey
	doc.Id = docId
	return doc
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
