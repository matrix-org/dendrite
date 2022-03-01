package caching

import (
	"fmt"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func NewInMemoryLRUCache(enablePrometheus bool) (*Caches, error) {
	roomVersions, err := NewInMemoryLRUCachePartition(
		RoomVersionCacheName,
		RoomVersionCacheMutable,
		RoomVersionCacheMaxEntries,
		RoomVersionCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	serverKeys, err := NewInMemoryLRUCachePartition(
		ServerKeyCacheName,
		ServerKeyCacheMutable,
		ServerKeyCacheMaxEntries,
		ServerKeyCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	roomServerRoomIDs, err := NewInMemoryLRUCachePartition(
		RoomServerRoomIDsCacheName,
		RoomServerRoomIDsCacheMutable,
		RoomServerRoomIDsCacheMaxEntries,
		RoomServerRoomIDsCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	roomInfos, err := NewInMemoryLRUCachePartition(
		RoomInfoCacheName,
		RoomInfoCacheMutable,
		RoomInfoCacheMaxEntries,
		RoomInfoCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	federationEvents, err := NewInMemoryLRUCachePartition(
		FederationEventCacheName,
		FederationEventCacheMutable,
		FederationEventCacheMaxEntries,
		FederationEventCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	spaceRooms, err := NewInMemoryLRUCachePartition(
		SpaceSummaryRoomsCacheName,
		SpaceSummaryRoomsCacheMutable,
		SpaceSummaryRoomsCacheMaxEntries,
		SpaceSummaryRoomsCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	go cacheCleaner(
		roomVersions, serverKeys, roomServerRoomIDs,
		roomInfos, federationEvents, spaceRooms,
	)
	return &Caches{
		RoomVersions:      roomVersions,
		ServerKeys:        serverKeys,
		RoomServerRoomIDs: roomServerRoomIDs,
		RoomInfos:         roomInfos,
		FederationEvents:  federationEvents,
		SpaceSummaryRooms: spaceRooms,
	}, nil
}

func cacheCleaner(caches ...*InMemoryLRUCachePartition) {
	for {
		time.Sleep(time.Minute)
		for _, cache := range caches {
			// Hold onto the last 10% of the cache entries, since
			// otherwise a quiet period might cause us to evict all
			// cache entries entirely.
			if cache.lru.Len() > cache.maxEntries/10 {
				cache.lru.RemoveOldest()
			}
		}
	}
}

type InMemoryLRUCachePartition struct {
	name       string
	mutable    bool
	maxEntries int
	maxAge     time.Duration
	lru        *lru.Cache
}

type inMemoryLRUCacheEntry struct {
	value   interface{}
	created time.Time
}

func NewInMemoryLRUCachePartition(name string, mutable bool, maxEntries int, maxAge time.Duration, enablePrometheus bool) (*InMemoryLRUCachePartition, error) {
	var err error
	cache := InMemoryLRUCachePartition{
		name:       name,
		mutable:    mutable,
		maxEntries: maxEntries,
		maxAge:     maxAge,
	}
	cache.lru, err = lru.New(maxEntries)
	if err != nil {
		return nil, err
	}
	if enablePrometheus {
		promauto.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "dendrite",
			Subsystem: "caching_in_memory_lru",
			Name:      name,
		}, func() float64 {
			return float64(cache.lru.Len())
		})
	}
	return &cache, nil
}

func (c *InMemoryLRUCachePartition) Set(key string, value interface{}) {
	if !c.mutable {
		if peek, ok := c.lru.Peek(key); ok {
			if entry, ok := peek.(*inMemoryLRUCacheEntry); ok && entry.value != value {
				panic(fmt.Sprintf("invalid use of immutable cache tries to mutate existing value of %q", key))
			}
		}
	}
	c.lru.Add(key, &inMemoryLRUCacheEntry{
		value:   value,
		created: time.Now(),
	})
}

func (c *InMemoryLRUCachePartition) Unset(key string) {
	if !c.mutable {
		panic(fmt.Sprintf("invalid use of immutable cache tries to unset value of %q", key))
	}
	c.lru.Remove(key)
}

func (c *InMemoryLRUCachePartition) Get(key string) (value interface{}, ok bool) {
	v, ok := c.lru.Get(key)
	if !ok {
		return nil, false
	}
	entry, ok := v.(*inMemoryLRUCacheEntry)
	switch {
	case ok && c.maxAge == CacheNoMaxAge:
		return entry.value, ok // There's no maximum age policy
	case ok && time.Since(entry.created) < c.maxAge:
		return entry.value, ok // The value for the key isn't stale
	default:
		// Either the key was found and it was stale, or the key
		// wasn't found at all
		c.lru.Remove(key)
		return nil, false
	}
}
