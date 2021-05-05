package api

import "context"

type PushserverInternalAPI interface {
	QueryExample(
		ctx context.Context,
		request *QueryExampleRequest,
		response *QueryExampleResponse,
	) error
}

type QueryExampleRequest struct{}
type QueryExampleResponse struct{}
