package caching

import (
	"fmt"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func NewRistrettoCache(enablePrometheus bool) (*Caches, error) {
	roomVersions, err := NewRistrettoCachePartition(
		RoomVersionCacheName,
		RoomVersionCacheMutable,
		RoomVersionCacheMaxEntries,
		RoomVersionCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	serverKeys, err := NewRistrettoCachePartition(
		ServerKeyCacheName,
		ServerKeyCacheMutable,
		ServerKeyCacheMaxEntries,
		ServerKeyCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	roomServerRoomIDs, err := NewRistrettoCachePartition(
		RoomServerRoomIDsCacheName,
		RoomServerRoomIDsCacheMutable,
		RoomServerRoomIDsCacheMaxEntries,
		RoomServerRoomIDsCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	roomInfos, err := NewRistrettoCachePartition(
		RoomInfoCacheName,
		RoomInfoCacheMutable,
		RoomInfoCacheMaxEntries,
		RoomInfoCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	federationEvents, err := NewRistrettoCachePartition(
		FederationEventCacheName,
		FederationEventCacheMutable,
		FederationEventCacheMaxEntries,
		FederationEventCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}
	spaceRooms, err := NewRistrettoCachePartition(
		SpaceSummaryRoomsCacheName,
		SpaceSummaryRoomsCacheMutable,
		SpaceSummaryRoomsCacheMaxEntries,
		SpaceSummaryRoomsCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}

	lazyLoadCache, err := NewRistrettoCachePartition(
		LazyLoadCacheName,
		LazyLoadCacheMutable,
		LazyLoadCacheMaxEntries,
		LazyLoadCacheMaxAge,
		enablePrometheus,
	)
	if err != nil {
		return nil, err
	}

	return &Caches{
		RoomVersions:      roomVersions,
		ServerKeys:        serverKeys,
		RoomServerRoomIDs: roomServerRoomIDs,
		RoomInfos:         roomInfos,
		FederationEvents:  federationEvents,
		SpaceSummaryRooms: spaceRooms,
		LazyLoading:       lazyLoadCache,
	}, nil
}

type RistrettoCachePartition struct {
	name       string
	mutable    bool
	maxEntries int
	maxAge     time.Duration
	ristretto  *ristretto.Cache
}

func NewRistrettoCachePartition(name string, mutable bool, maxEntries int, maxAge time.Duration, enablePrometheus bool) (*RistrettoCachePartition, error) {
	var err error
	cache := RistrettoCachePartition{
		name:       name,
		mutable:    mutable,
		maxEntries: maxEntries,
		maxAge:     maxAge,
	}
	cache.ristretto, err = ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     1 << 27,
		BufferItems: 64,
		Metrics:     true,
	})
	if err != nil {
		return nil, err
	}
	if enablePrometheus {
		promauto.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "dendrite",
			Subsystem: "caching_ristretto",
			Name:      name,
		}, func() float64 {
			return float64(cache.ristretto.Metrics.Ratio())
		})
	}
	return &cache, nil
}

func (c *RistrettoCachePartition) Set(key string, value interface{}) {
	if !c.mutable {
		if peek, ok := c.ristretto.Get(key); ok {
			if entry, ok := peek.(*inMemoryLRUCacheEntry); ok && entry.value != value {
				panic(fmt.Sprintf("invalid use of immutable cache tries to mutate existing value of %q", key))
			}
		}
	}
	c.ristretto.SetWithTTL(key, value, 1, c.maxAge)
}

func (c *RistrettoCachePartition) Unset(key string) {
	if !c.mutable {
		panic(fmt.Sprintf("invalid use of immutable cache tries to unset value of %q", key))
	}
	c.ristretto.Del(key)
}

func (c *RistrettoCachePartition) Get(key string) (value interface{}, ok bool) {
	return c.ristretto.Get(key)
}
