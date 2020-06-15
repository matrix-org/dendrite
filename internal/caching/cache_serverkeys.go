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
	GetServerKey(request gomatrixserverlib.PublicKeyLookupRequest, timestamp gomatrixserverlib.Timestamp) (response gomatrixserverlib.PublicKeyLookupResult, ok bool)
	StoreServerKey(request gomatrixserverlib.PublicKeyLookupRequest, response gomatrixserverlib.PublicKeyLookupResult)
}

func (c Caches) GetServerKey(
	request gomatrixserverlib.PublicKeyLookupRequest,
	timestamp gomatrixserverlib.Timestamp,
) (gomatrixserverlib.PublicKeyLookupResult, bool) {
	key := fmt.Sprintf("%s/%s", request.ServerName, request.KeyID)
	now := gomatrixserverlib.AsTimestamp(time.Now())
	val, found := c.ServerKeys.Get(key)
	if found && val != nil {
		if keyLookupResult, ok := val.(gomatrixserverlib.PublicKeyLookupResult); ok {
			if !keyLookupResult.WasValidAt(timestamp, true) {
				// The key wasn't valid at the requested timestamp so don't
				// return it. The caller will have to work out what to do.
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
