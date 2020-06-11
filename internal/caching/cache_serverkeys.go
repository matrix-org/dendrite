package caching

import (
	"fmt"
	"time"

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
	now := gomatrixserverlib.AsTimestamp(time.Now())
	val, found := c.ServerKeys.Get(key)
	if found && val != nil {
		if keyLookupResult, ok := val.(gomatrixserverlib.PublicKeyLookupResult); ok {
			if !keyLookupResult.WasValidAt(now, true) {
				// We appear to be past the key validity so don't return this
				// with the results. This ensures that the cache doesn't return
				// values that are not useful to us.
				c.ServerKeys.Unset(key)
				return gomatrixserverlib.PublicKeyLookupResult{}, false
			}
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
