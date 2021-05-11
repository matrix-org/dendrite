package cosmosdbapi

import (
	"fmt"

)

func GetDocumentId(tenantName string, collectionName string, id string) string {
	return fmt.Sprintf("%s,%s,%s", collectionName, tenantName, id)
}

func GetPartitionKey(tenantName string, collectionName string) string {
	return fmt.Sprintf("%s,%s", collectionName, tenantName)
}