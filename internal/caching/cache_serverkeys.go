package caching

import (
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

// ServerKeyCache contains the subset of functions needed for
// a server key cache.
type ServerKeyCache interface {
	// request -> timestamp is emulating gomatrixserverlib.FetchKeys:
	// https://github.com/matrix-org/gomatrixserverlib/blob/f69539c86ea55d1e2cc76fd8e944e2d82d30397c/keyring.go#L95
	// The timestamp should be the timestamp of the event that is being
	// verified. We will not return keys from the cache that are not valid
	// at this timestamp.
	GetServerKey(request gomatrixserverlib.PublicKeyLookupRequest, timestamp gomatrixserverlib.Timestamp) (response gomatrixserverlib.PublicKeyLookupResult, ok bool)

	// request -> result is emulating gomatrixserverlib.StoreKeys:
	// https://github.com/matrix-org/gomatrixserverlib/blob/f69539c86ea55d1e2cc76fd8e944e2d82d30397c/keyring.go#L112
	StoreServerKey(request gomatrixserverlib.PublicKeyLookupRequest, response gomatrixserverlib.PublicKeyLookupResult)
}

func (c Caches) GetServerKey(
	request gomatrixserverlib.PublicKeyLookupRequest,
	timestamp gomatrixserverlib.Timestamp,
) (gomatrixserverlib.PublicKeyLookupResult, bool) {
	key := fmt.Sprintf("%s/%s", request.ServerName, request.KeyID)
	val, found := c.ServerKeys.Get(key)
	if found && !val.WasValidAt(timestamp, true) {
		// The key wasn't valid at the requested timestamp so don't
		// return it. The caller will have to work out what to do.
		c.ServerKeys.Unset(key)
		return gomatrixserverlib.PublicKeyLookupResult{}, false
	}
	return val, found
}

func (c Caches) StoreServerKey(
	request gomatrixserverlib.PublicKeyLookupRequest,
	response gomatrixserverlib.PublicKeyLookupResult,
) {
	key := fmt.Sprintf("%s/%s", request.ServerName, request.KeyID)
	c.ServerKeys.Set(key, response)
}
