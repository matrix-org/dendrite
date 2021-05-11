package cosmosdbapi

import (
	"fmt"

)

func GetCollectionName(databaseName string, tableName string) string {
	return fmt.Sprintf("matrix_%s_%s", databaseName, tableName)
}