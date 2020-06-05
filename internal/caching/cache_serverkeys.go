package caching

import (
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

const (
	ServerKeyCacheName       = "server_key"
	ServerKeyCacheMaxEntries = 4096
	ServerKeyCacheMutable    = true
)

// ServerKeyCache contains the subset of functions needed for
// a server key cache.
type ServerKeyCache interface {
	GetServerKey(request gomatrixserverlib.PublicKeyLookupRequest) (response gomatrixserverlib.PublicKeyLookupResult, ok bool)
	StoreServerKey(request gomatrixserverlib.PublicKeyLookupRequest, response gomatrixserverlib.PublicKeyLookupResult)
}

func (c Caches) GetServerKey(
	request gomatrixserverlib.PublicKeyLookupRequest,
) (gomatrixserverlib.PublicKeyLookupResult, bool) {
	key := fmt.Sprintf("%s/%s", request.ServerName, request.KeyID)
	val, found := c.ServerKeys.Get(key)
	if found && val != nil {
		if keyLookupResult, ok := val.(gomatrixserverlib.PublicKeyLookupResult); ok {
			return keyLookupResult, true
		}
	}
	return gomatrixserverlib.PublicKeyLookupResult{}, false
}

func (c Caches) StoreServerKey(
	request gomatrixserverlib.PublicKeyLookupRequest,
	response gomatrixserverlib.PublicKeyLookupResult,
) {
	key := fmt.Sprintf("%s/%s", request.ServerName, request.KeyID)
	c.ServerKeys.Set(key, response)
}
