package caching

import (
	"fmt"
	"time"
)

const (
	LazyLoadCacheName       = "lazy_load_members"
	LazyLoadCacheMaxEntries = 128
	LazyLoadCacheMutable    = true
	LazyLoadCacheMaxAge     = time.Minute * 30
)

type LazyLoadCache struct {
	*InMemoryLRUCachePartition
}

// NewLazyLoadCache creates a new InMemoryLRUCachePartition.
func NewLazyLoadCache() (*LazyLoadCache, error) {
	cache, err := NewInMemoryLRUCachePartition(
		LazyLoadCacheName,
		LazyLoadCacheMutable,
		LazyLoadCacheMaxEntries,
		LazyLoadCacheMaxAge,
		false,
	)
	if err != nil {
		return nil, err
	}
	go cacheCleaner(cache)
	return &LazyLoadCache{cache}, err
}

func (c *LazyLoadCache) StoreLazyLoadedUser(reqUser, deviceID, roomID, userID, eventID string) {
	cacheKey := fmt.Sprintf("%s/%s/%s/%s", reqUser, deviceID, roomID, userID)
	c.Set(cacheKey, eventID)
}

func (c *LazyLoadCache) IsLazyLoadedUserCached(reqUser, deviceID, roomID, userID string) (string, bool) {
	cacheKey := fmt.Sprintf("%s/%s/%s/%s", reqUser, deviceID, roomID, userID)
	val, ok := c.Get(cacheKey)
	if !ok {
		return "", ok
	}
	return val.(string), ok
}
