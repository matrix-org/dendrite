package api

import (
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/common/caching"
	"github.com/matrix-org/gomatrixserverlib"
)

type ServerKeyInternalAPI interface {
	gomatrixserverlib.KeyDatabase
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
