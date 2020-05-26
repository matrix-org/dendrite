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
