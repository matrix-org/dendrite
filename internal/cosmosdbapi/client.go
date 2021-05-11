package cosmosdbapi

import (
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
