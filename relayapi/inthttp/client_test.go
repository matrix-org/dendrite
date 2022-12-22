package inthttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/relayapi/api"
	"github.com/stretchr/testify/assert"
)

func TestRelayAPIClientNil(t *testing.T) {
	_, err := NewRelayAPIClient("", nil, nil)
	assert.Error(t, err)
}

func TestRelayAPIClientPerormSync(t *testing.T) {
	// Start a local HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/api"+RelayAPIPerformRelayServerSyncPath, req.URL.String())
	}))
	defer server.Close()

	cl, err := NewRelayAPIClient(server.URL, server.Client(), nil)
	assert.NoError(t, err)
	assert.NotNil(t, cl)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cl.PerformRelayServerSync(ctx, &api.PerformRelayServerSyncRequest{}, &api.PerformRelayServerSyncResponse{})
}

func TestRelayAPIClientStore(t *testing.T) {
	// Start a local HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/api"+RelayAPIPerformStoreAsyncPath, req.URL.String())
	}))
	defer server.Close()

	cl, err := NewRelayAPIClient(server.URL, server.Client(), nil)
	assert.NoError(t, err)
	assert.NotNil(t, cl)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cl.PerformStoreAsync(ctx, &api.PerformStoreAsyncRequest{}, &api.PerformStoreAsyncResponse{})
}

func TestRelayAPIClientQuery(t *testing.T) {
	// Start a local HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/api"+RelayAPIQueryAsyncTransactionsPath, req.URL.String())
	}))
	defer server.Close()

	cl, err := NewRelayAPIClient(server.URL, server.Client(), nil)
	assert.NoError(t, err)
	assert.NotNil(t, cl)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cl.QueryAsyncTransactions(ctx, &api.QueryAsyncTransactionsRequest{}, &api.QueryAsyncTransactionsResponse{})
}
