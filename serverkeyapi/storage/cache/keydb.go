package cache

import (
	"context"
	"errors"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/gomatrixserverlib"
)

// A Database implements gomatrixserverlib.KeyDatabase and is used to store
// the public keys for other matrix servers.
type KeyDatabase struct {
	inner gomatrixserverlib.KeyDatabase
	cache caching.ServerKeyCache
}

func NewKeyDatabase(inner gomatrixserverlib.KeyDatabase, cache caching.ServerKeyCache) (*KeyDatabase, error) {
	if inner == nil {
		return nil, errors.New("inner database can't be nil")
	}
	if cache == nil {
		return nil, errors.New("cache can't be nil")
	}
	return &KeyDatabase{
		inner: inner,
		cache: cache,
	}, nil
}

// FetcherName implements KeyFetcher
func (d KeyDatabase) FetcherName() string {
	return "InMemoryKeyCache"
}

// FetchKeys implements gomatrixserverlib.KeyDatabase
func (d *KeyDatabase) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	results := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult)
	for req, ts := range requests {
		if res, cached := d.cache.GetServerKey(req, ts); cached {
			results[req] = res
			delete(requests, req)
		}
	}
	fromDB, err := d.inner.FetchKeys(ctx, requests)
	if err != nil {
		return results, err
	}
	for req, res := range fromDB {
		results[req] = res
		d.cache.StoreServerKey(req, res)
	}
	return results, nil
}

// StoreKeys implements gomatrixserverlib.KeyDatabase
func (d *KeyDatabase) StoreKeys(
	ctx context.Context,
	keyMap map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	for req, res := range keyMap {
		d.cache.StoreServerKey(req, res)
	}
	return d.inner.StoreKeys(ctx, keyMap)
}
