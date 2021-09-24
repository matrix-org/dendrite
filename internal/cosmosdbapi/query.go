package cosmosdbapi

import (
	cosmosapi "github.com/vippsas/go-cosmosdb/cosmosapi"
)

func getQuery(qry string, params map[string]interface{}) cosmosapi.Query {
	qryParams := []cosmosapi.QueryParam{}
	for key, value := range params {
		qryParam := cosmosapi.QueryParam{
			Name:  key,
			Value: value,
		}
		qryParams = append(qryParams, qryParam)
	}
	return cosmosapi.Query{
		Query:  qry,
		Params: qryParams,
	}
}
