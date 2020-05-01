package api

import (
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/common/caching"
	fsInputAPI "github.com/matrix-org/dendrite/federationsender/api"
)

type httpRoomserverInternalAPI struct {
	roomserverURL  string
	httpClient     *http.Client
	fsAPI          fsInputAPI.FederationSenderInternalAPI
	immutableCache caching.ImmutableCache
}

// NewRoomserverInputAPIHTTP creates a RoomserverInputAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewRoomserverInternalAPIHTTP(
	roomserverURL string,
	httpClient *http.Client,
	//fsInputAPI fsAPI.FederationSenderInternalAPI,
	immutableCache caching.ImmutableCache,
) (RoomserverInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewRoomserverInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpRoomserverInternalAPI{
		roomserverURL:  roomserverURL,
		httpClient:     httpClient,
		immutableCache: immutableCache,
	}, nil
}

// SetFederationSenderInputAPI passes in a federation sender input API reference
// so that we can avoid the chicken-and-egg problem of both the roomserver input API
// and the federation sender input API being interdependent.
func (h *httpRoomserverInternalAPI) SetFederationSenderAPI(fsAPI fsInputAPI.FederationSenderInternalAPI) {
	h.fsAPI = fsAPI
}
