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

func (c *LazyLoadCache) StoreLazyLoadedUser(reqUser, deviceID, roomID, userID string) {
	cacheKey := fmt.Sprintf("%s/%s/%s/%s", reqUser, deviceID, roomID, userID)
	c.Set(cacheKey, true)
}

func (c *LazyLoadCache) IsLazyLoadedUserCached(reqUser, deviceID, roomID, userID string) bool {
	cacheKey := fmt.Sprintf("%s/%s/%s/%s", reqUser, deviceID, roomID, userID)
	_, ok := c.Get(cacheKey)
	return ok
}
