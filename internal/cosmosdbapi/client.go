package cosmosdbapi

import (
	"context"
	"errors"
	"strings"
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

func PerformQuery(ctx context.Context,
	conn CosmosConnection,
	databaseName string,
	containerName string,
	partitonKey string,
	qryString string,
	params map[string]interface{},
	response interface{}) error {
	err := validateQuery(qryString)
	if err != nil {
		return err
	}
	optionsQry := getQueryDocumentsOptions(partitonKey)
	var query = getQuery(qryString, params)
	_, err = GetClient(conn).QueryDocuments(
		ctx,
		databaseName,
		containerName,
		query,
		&response,
		optionsQry)
	return err
}

func PerformQueryAllPartitions(ctx context.Context,
	conn CosmosConnection,
	databaseName string,
	containerName string,
	qryString string,
	params map[string]interface{},
	response interface{}) error {
	err := validateQueryAllPartitions(qryString)
	if err != nil {
		return err
	}
	var optionsQry = getQueryAllPartitionsDocumentsOptions()
	var query = getQuery(qryString, params)
	_, err = GetClient(conn).QueryDocuments(
		ctx,
		databaseName,
		containerName,
		query,
		&response,
		optionsQry)

	// When there are no Rows we seem to get the generic Bad Req JSON error
	if err != nil {
		// return nil, err
	}

	return nil
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

func UpdateDocument(ctx context.Context,
	conn CosmosConnection,
	databaseName string,
	containerName string,
	partitionKey string,
	eTag string,
	docId string,
	document interface{},
) (*interface{}, error) {
	optionsReplace := getReplaceDocumentOptions(partitionKey, eTag)
	_, _, err := GetClient(conn).ReplaceDocument(
		ctx,
		databaseName,
		containerName,
		docId,
		&document,
		optionsReplace)
	return &document, err
}

func validateQuery(qryString string) error {
	if len(qryString) == 0 {
		return errors.New("qryString was nil")
	}
	if !strings.Contains(qryString, " c._cn = ") {
		return errors.New("qryString must contain [ c._cn = ] ")
	}
	return nil
}

func validateQueryAllPartitions(qryString string) error {
	err := validateQuery(qryString)
	if err != nil {
		return err
	}
	if strings.Contains(qryString, " top ") {
		return errors.New("qryString contains [ top ] ")
	}
	if strings.Contains(qryString, " order by ") {
		return errors.New("qryString contains [ order by ] ")
	}
	if !strings.Contains(qryString, " c._sid = ") {
		return errors.New("qryString must contain [ c._sid = ] ")
	}
	return nil
}
