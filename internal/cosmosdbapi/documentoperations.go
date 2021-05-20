package cosmosdbapi

import (
	cosmosapi "github.com/vippsas/go-cosmosdb/cosmosapi"
)

func GetCreateDocumentOptions(pk string) cosmosapi.CreateDocumentOptions {
	return cosmosapi.CreateDocumentOptions{
		IsUpsert:          false,
		PartitionKeyValue: pk,
	}
}

func GetUpsertDocumentOptions(pk string) cosmosapi.CreateDocumentOptions {
	return cosmosapi.CreateDocumentOptions{
		IsUpsert:          true,
		PartitionKeyValue: pk,
	}
}

func GetQueryDocumentsOptions(pk string) cosmosapi.QueryDocumentsOptions {
	return cosmosapi.QueryDocumentsOptions{
		PartitionKeyValue: pk,
		IsQuery:           true,
		ContentType:       cosmosapi.QUERY_CONTENT_TYPE,
	}
}

func GetQueryAllPartitionsDocumentsOptions() cosmosapi.QueryDocumentsOptions {
	return cosmosapi.QueryDocumentsOptions{
		IsQuery:              true,
		EnableCrossPartition: true,
		ContentType:          cosmosapi.QUERY_CONTENT_TYPE,
	}
}

func GetGetDocumentOptions(pk string) cosmosapi.GetDocumentOptions {
	return cosmosapi.GetDocumentOptions{
		PartitionKeyValue: pk,
	}
}

func GetReplaceDocumentOptions(pk string, etag string) cosmosapi.ReplaceDocumentOptions {
	return cosmosapi.ReplaceDocumentOptions{
		PartitionKeyValue: pk,
		IfMatch:           etag,
	}
}

func GetDeleteDocumentOptions(pk string) cosmosapi.DeleteDocumentOptions {
	return cosmosapi.DeleteDocumentOptions{
		PartitionKeyValue: pk,
	}
}
