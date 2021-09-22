package cosmosdbapi

import (
	"fmt"
	"strings"
)

type CosmosDocument struct {
	Id        string `json:"id"`
	Pk        string `json:"_pk"`
	Tn        string `json:"_sid"`
	Cn        string `json:"_cn"`
	Ct        string `json:"_ct"`
	Ut        string `json:"_ut"`
	ETag      string `json:"_etag"`
	Timestamp int64  `json:"_ts"`
}

func removeSpecialChars(docId string) string {
	// The following characters are restricted and cannot be used in the Id property: '/', '\', '?', '#'
	invalidChars := [4]string{"/", "\\", "?", "#"}
	replaceChar := ","
	result := docId
	for _, invalidChar := range invalidChars {
		result = strings.ReplaceAll(result, invalidChar, replaceChar)
	}
	return result
}

func GetDocumentId(tenantName string, collectionName string, id string) string {
	safeId := removeSpecialChars(id)
	return fmt.Sprintf("%s,%s,%s", collectionName, tenantName, safeId)
}

func GetPartitionKeyByCollection(tenantName string, collectionName string) string {
	return fmt.Sprintf("%s,%s", collectionName, tenantName)
}

func GetPartitionKeyByUniqueId(tenantName string, collectionName string, uniqueId string) string {
	return fmt.Sprintf("%s,%s,%s", collectionName, tenantName, uniqueId)
}
