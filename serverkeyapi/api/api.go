package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/gomatrixserverlib"
)

type ServerKeyInternalAPI interface {
	gomatrixserverlib.KeyDatabase

	KeyRing() *gomatrixserverlib.KeyRing

	InputPublicKeys(
		ctx context.Context,
		request *InputPublicKeysRequest,
		response *InputPublicKeysResponse,
	) error

	QueryPublicKeys(
		ctx context.Context,
		request *QueryPublicKeysRequest,
		response *QueryPublicKeysResponse,
	) error
}

// NewRoomserverInputAPIHTTP creates a RoomserverInputAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewServerKeyInternalAPIHTTP(
	serverKeyAPIURL string,
	httpClient *http.Client,
	immutableCache caching.ImmutableCache,
) (ServerKeyInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewRoomserverInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpServerKeyInternalAPI{
		serverKeyAPIURL: serverKeyAPIURL,
		httpClient:      httpClient,
		immutableCache:  immutableCache,
	}, nil
}

type httpServerKeyInternalAPI struct {
	ServerKeyInternalAPI

	serverKeyAPIURL string
	httpClient      *http.Client
	immutableCache  caching.ImmutableCache
}

func (s *httpServerKeyInternalAPI) KeyRing() *gomatrixserverlib.KeyRing {
	// This is a bit of a cheat - we tell gomatrixserverlib that this API is
	// both the key database and the key fetcher. While this does have the
	// rather unfortunate effect of preventing gomatrixserverlib from handling
	// key fetchers directly, we can at least reimplement this behaviour on
	// the other end of the API.
	return &gomatrixserverlib.KeyRing{
		KeyDatabase: s,
		KeyFetchers: []gomatrixserverlib.KeyFetcher{s},
	}
}

func (s *httpServerKeyInternalAPI) FetcherName() string {
	return "httpServerKeyInternalAPI"
}

func (s *httpServerKeyInternalAPI) StoreKeys(
	ctx context.Context,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	request := InputPublicKeysRequest{
		Keys: make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult),
	}
	response := InputPublicKeysResponse{}
	for req, res := range results {
		request.Keys[req] = res
		s.immutableCache.StoreServerKey(req, res)
	}
	return s.InputPublicKeys(ctx, &request, &response)
}

func (s *httpServerKeyInternalAPI) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	result := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult)
	request := QueryPublicKeysRequest{
		Requests: make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp),
	}
	response := QueryPublicKeysResponse{
		Results: make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult),
	}
	for req, ts := range requests {
		if res, ok := s.immutableCache.GetServerKey(req); ok {
			result[req] = res
			continue
		}
		request.Requests[req] = ts
	}
	err := s.QueryPublicKeys(ctx, &request, &response)
	if err != nil {
		return nil, err
	}
	for req, res := range response.Results {
		result[req] = res
		s.immutableCache.StoreServerKey(req, res)
	}
	return result, nil
}
