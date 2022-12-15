package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/relayapi/api"
)

// HTTP paths for the internal HTTP API
const (
	RelayAPIPerformRelayServerSyncPath = "/relayapi/performRelayServerSync"
	RelayAPIPerformStoreAsyncPath      = "/relayapi/performStoreAsync"
	RelayAPIQueryAsyncTransactionsPath = "/relayapi/queryAsyncTransactions"
)

// NewRelayAPIClient creates a RelayInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewRelayAPIClient(relayapiURL string, httpClient *http.Client, cache caching.ServerKeyCache) (api.RelayInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewRelayInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpRelayInternalAPI{
		relayAPIURL: relayapiURL,
		httpClient:  httpClient,
		cache:       cache,
	}, nil
}

type httpRelayInternalAPI struct {
	relayAPIURL string
	httpClient  *http.Client
	cache       caching.ServerKeyCache
}

func (h *httpRelayInternalAPI) PerformRelayServerSync(
	ctx context.Context,
	request *api.PerformRelayServerSyncRequest,
	response *api.PerformRelayServerSyncResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformRelayServerSync", h.relayAPIURL+RelayAPIPerformRelayServerSyncPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRelayInternalAPI) PerformStoreAsync(
	ctx context.Context,
	request *api.PerformStoreAsyncRequest,
	response *api.PerformStoreAsyncResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformStoreAsync", h.relayAPIURL+RelayAPIPerformStoreAsyncPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRelayInternalAPI) QueryAsyncTransactions(
	ctx context.Context,
	request *api.QueryAsyncTransactionsRequest,
	response *api.QueryAsyncTransactionsResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryAsyncTransactions", h.relayAPIURL+RelayAPIQueryAsyncTransactionsPath,
		h.httpClient, ctx, request, response,
	)
}
