package api

import (
	"context"
	"net/http"

	"github.com/matrix-org/dendrite/common/caching"
	"github.com/matrix-org/gomatrixserverlib"
)

type httpServerKeyInternalAPI struct {
	ServerKeyInternalAPI

	serverKeyAPIURL string
	httpClient      *http.Client
	immutableCache  caching.ImmutableCache
}

func (s *httpServerKeyInternalAPI) KeyRing() *gomatrixserverlib.KeyRing {
	return &gomatrixserverlib.KeyRing{
		KeyDatabase: s,
		KeyFetchers: []gomatrixserverlib.KeyFetcher{s},
	}
}

func (s *httpServerKeyInternalAPI) StoreKeys(
	ctx context.Context,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	return nil
}

func (s *httpServerKeyInternalAPI) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	return nil, nil
}
