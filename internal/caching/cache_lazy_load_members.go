package caching

import (
	"fmt"
	"time"

	userapi "github.com/matrix-org/dendrite/userapi/api"
)

const (
	LazyLoadCacheName           = "lazy_load_members"
	LazyLoadCacheMaxEntries     = 128
	LazyLoadCacheMaxUserEntries = 128
	LazyLoadCacheMutable        = true
	LazyLoadCacheMaxAge         = time.Minute * 30
)

type LazyLoadCache struct {
	// InMemoryLRUCachePartition containing other InMemoryLRUCachePartitions
	// with the actual cached members
	userCaches *InMemoryLRUCachePartition
}

// NewLazyLoadCache creates a new LazyLoadCache.
func NewLazyLoadCache() (*LazyLoadCache, error) {
	cache, err := NewInMemoryLRUCachePartition(
		LazyLoadCacheName,
		LazyLoadCacheMutable,
		LazyLoadCacheMaxEntries,
		LazyLoadCacheMaxAge,
		true,
	)
	if err != nil {
		return nil, err
	}
	go cacheCleaner(cache)
	return &LazyLoadCache{
		userCaches: cache,
	}, nil
}

func (c *LazyLoadCache) lazyLoadCacheForUser(device *userapi.Device) (*InMemoryLRUCachePartition, error) {
	cacheName := fmt.Sprintf("%s/%s", device.UserID, device.ID)
	userCache, ok := c.userCaches.Get(cacheName)
	if ok && userCache != nil {
		if cache, ok := userCache.(*InMemoryLRUCachePartition); ok {
			return cache, nil
		}
	}
	cache, err := NewInMemoryLRUCachePartition(
		LazyLoadCacheName,
		LazyLoadCacheMutable,
		LazyLoadCacheMaxUserEntries,
		LazyLoadCacheMaxAge,
		false,
	)
	if err != nil {
		return nil, err
	}
	c.userCaches.Set(cacheName, cache)
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
