package cosmosdbutil

import (
	"strconv"
	"strings"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/setup/config"
)

const accountEndpointName = "AccountEndpoint"
const accountKeyName = "AccountKey"
const databaseName = "DatabaseName"
const containerName = "ContainerName"
const tenantName = "TenantName"
const disableCertificateValidationName = "DisableCertificateValidation"

func getConnectionString(d *config.DataSource) config.DataSource {
	connString := string(*d)
	return config.DataSource(strings.Replace(connString, "cosmosdb:", "", 1))
}

func getConnectionProperties(connectionString string) map[string]string {
	connectionItemsRaw := strings.Split(connectionString, ";")
	connectionItems := map[string]string{}
	for _, item := range connectionItemsRaw {
		if len(item) > 0 {
			itemSplit := strings.SplitN(item, "=", 2)
			connectionItems[itemSplit[0]] = itemSplit[1]
		}
	}
	return connectionItems
}

func GetCosmosConnection(d *config.DataSource) cosmosdbapi.CosmosConnection {
	connString := getConnectionString(d)
	connMap := getConnectionProperties(string(connString))
	accountEndpoint := connMap[accountEndpointName]
	accountKey := connMap[accountKeyName]
	value, ok := connMap[disableCertificateValidationName]
	disableCertificateValidation := false
	if ok {
		valueBool, err := strconv.ParseBool(value)
		if err == nil {
			disableCertificateValidation = valueBool
		}
	}
	return cosmosdbapi.GetCosmosConnection(accountEndpoint, accountKey, disableCertificateValidation)
}

func GetCosmosConfig(d *config.DataSource) cosmosdbapi.CosmosConfig {
	connString := getConnectionString(d)
	connMap := getConnectionProperties(string(connString))
	database := connMap[databaseName]
	container := connMap[containerName]
	tenant := connMap[tenantName]
	return cosmosdbapi.CosmosConfig{
		DatabaseName:  database,
		ContainerName: container,
		TenantName:    tenant,
	}
}
