package caching

import (
	"fmt"
	"time"

	userapi "github.com/matrix-org/dendrite/userapi/api"
)

const (
	LazyLoadCacheName       = "lazy_load_members"
	LazyLoadCacheMaxEntries = 128
	LazyLoadCacheMutable    = true
	LazyLoadCacheMaxAge     = time.Minute * 30
)

type LazyLoadCache struct {
	// Mapping from userID/deviceID to InMemoryLRUCachePartition
	userCaches map[string]*InMemoryLRUCachePartition
}

// NewLazyLoadCache creates a new LazyLoadCache.
func NewLazyLoadCache() *LazyLoadCache {
	return &LazyLoadCache{
		userCaches: make(map[string]*InMemoryLRUCachePartition),
	}
}

func (c *LazyLoadCache) lazyLoadCacheForUser(device *userapi.Device) (*InMemoryLRUCachePartition, error) {
	cacheName := fmt.Sprintf("%s/%s", device.UserID, device.ID)
	cache, ok := c.userCaches[cacheName]
	if ok {
		return cache, nil
	}
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
	c.userCaches[cacheName] = cache
	go cacheCleaner(cache)
	return cache, nil
}

func (c *LazyLoadCache) StoreLazyLoadedUser(device *userapi.Device, roomID, userID, eventID string) {
	cache, err := c.lazyLoadCacheForUser(device)
	if err != nil {
		return
	}
	cacheKey := fmt.Sprintf("%s/%s/%s/%s", device.UserID, device.ID, roomID, userID)
	cache.Set(cacheKey, eventID)
}

func (c *LazyLoadCache) IsLazyLoadedUserCached(device *userapi.Device, roomID, userID string) (string, bool) {
	cache, err := c.lazyLoadCacheForUser(device)
	if err != nil {
		return "", false
	}

	cacheKey := fmt.Sprintf("%s/%s/%s/%s", device.UserID, device.ID, roomID, userID)
	val, ok := cache.Get(cacheKey)
	if !ok {
		return "", ok
	}
	return val.(string), ok
}
