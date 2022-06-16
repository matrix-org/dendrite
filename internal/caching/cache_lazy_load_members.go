package caching

import (
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type LazyLoadCache interface {
	StoreLazyLoadedUser(device *userapi.Device, roomID, userID, eventID string)
	IsLazyLoadedUserCached(device *userapi.Device, roomID, userID string) (string, bool)
}

func (c Caches) lazyLoadCacheForUser(device *userapi.Device) (*RistrettoCachePartition[string, any], error) {
	return nil, nil
	/*
		cacheName := fmt.Sprintf("%s/%s", device.UserID, device.ID)
		userCache, ok := c.LazyLoading.Get(cacheName)
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
		c.LazyLoading.Set(cacheName, cache)
		go cacheCleaner(cache)
		return cache, nil
	*/
}

func (c Caches) StoreLazyLoadedUser(device *userapi.Device, roomID, userID, eventID string) {
	/*
		cache, err := c.lazyLoadCacheForUser(device)
		if err != nil {
			return
		}
		cacheKey := fmt.Sprintf("%s/%s/%s/%s", device.UserID, device.ID, roomID, userID)
		cache.Set(cacheKey, eventID)
	*/
}

func (c Caches) IsLazyLoadedUserCached(device *userapi.Device, roomID, userID string) (string, bool) {
	return "", false
	/*
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
	*/
}
