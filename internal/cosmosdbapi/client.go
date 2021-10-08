package cosmosdbapi

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	cosmosapi "github.com/vippsas/go-cosmosdb/cosmosapi"
)

type CosmosConnection struct {
	Url                          string
	Key                          string
	DisableCertificateValidation bool
}

func GetCosmosConnection(accountEndpoint string, accountKey string, disableCertificateValidation bool) CosmosConnection {
	return CosmosConnection{
		Url:                          accountEndpoint,
		Key:                          accountKey,
		DisableCertificateValidation: disableCertificateValidation,
	}
}

func disableCertificateValidation() {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
}

func GetClient(conn CosmosConnection) *cosmosapi.Client {
	cfg := cosmosapi.Config{
		MasterKey: conn.Key,
	}
	if conn.DisableCertificateValidation {
		disableCertificateValidation()
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
