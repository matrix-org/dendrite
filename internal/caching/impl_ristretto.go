package caching

import (
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/dgraph-io/ristretto"
	"github.com/dgraph-io/ristretto/z"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func MustCreateCache(maxCost config.DataUnit, enablePrometheus bool) *ristretto.Cache {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e5,
		MaxCost:     int64(maxCost),
		BufferItems: 64,
		Metrics:     true,
		KeyToHash: func(key interface{}) (uint64, uint64) {
			return z.KeyToHash(key)
		},
	})
	if err != nil {
		panic(err)
	}
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "caching_ristretto",
		Name:      "ratio",
	}, func() float64 {
		return float64(cache.Metrics.Ratio())
	})
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "caching_ristretto",
		Name:      "cost",
	}, func() float64 {
		return float64(cache.Metrics.CostAdded() - cache.Metrics.CostEvicted())
	})
	return cache
}

const (
	roomVersionsCache byte = iota + 1
	serverKeysCache
	roomIDsCache
	roomEventsCache
	roomInfosCache
	federationPDUsCache
	federationEDUsCache
	spaceSummaryRoomsCache
	lazyLoadingCache
)

func NewRistrettoCache(maxCost config.DataUnit, enablePrometheus bool) (*Caches, error) {
	cache := MustCreateCache(maxCost, enablePrometheus)
	return &Caches{
		RoomVersions: &RistrettoCachePartition[string, gomatrixserverlib.RoomVersion]{
			cache:  cache,
			Prefix: roomVersionsCache,
			MaxAge: time.Hour,
		},
		ServerKeys: &RistrettoCachePartition[string, gomatrixserverlib.PublicKeyLookupResult]{
			cache:   cache,
			Prefix:  serverKeysCache,
			Mutable: true,
			MaxAge:  time.Hour,
		},
		RoomServerRoomIDs: &RistrettoCachePartition[int64, string]{
			cache:  cache,
			Prefix: roomIDsCache,
			MaxAge: time.Hour,
		},
		RoomServerEvents: &RistrettoCostedCachePartition[int64, *gomatrixserverlib.Event]{
			&RistrettoCachePartition[int64, *gomatrixserverlib.Event]{
				cache:  cache,
				Prefix: roomEventsCache,
				MaxAge: time.Hour,
			},
		},
		RoomInfos: &RistrettoCachePartition[string, types.RoomInfo]{
			cache:   cache,
			Prefix:  roomInfosCache,
			Mutable: true,
			MaxAge:  time.Hour,
		},
		FederationPDUs: &RistrettoCostedCachePartition[int64, *gomatrixserverlib.HeaderedEvent]{
			&RistrettoCachePartition[int64, *gomatrixserverlib.HeaderedEvent]{
				cache:   cache,
				Prefix:  federationPDUsCache,
				Mutable: true,
				MaxAge:  time.Hour / 2,
			},
		},
		FederationEDUs: &RistrettoCostedCachePartition[int64, *gomatrixserverlib.EDU]{
			&RistrettoCachePartition[int64, *gomatrixserverlib.EDU]{
				cache:   cache,
				Prefix:  federationEDUsCache,
				Mutable: true,
				MaxAge:  time.Hour / 2,
			},
		},
		SpaceSummaryRooms: &RistrettoCachePartition[string, gomatrixserverlib.MSC2946SpacesResponse]{
			cache:   cache,
			Prefix:  spaceSummaryRoomsCache,
			Mutable: true,
			MaxAge:  time.Hour,
		},
		LazyLoading: &RistrettoCachePartition[string, any]{ // TODO: type
			cache:   cache,
			Prefix:  lazyLoadingCache,
			Mutable: true,
			MaxAge:  time.Hour,
		},
	}, nil
}

type RistrettoCostedCachePartition[k keyable, v costable] struct {
	*RistrettoCachePartition[k, v]
}

func (c *RistrettoCostedCachePartition[K, V]) Set(key K, value V) {
	cost := value.CacheCost()
	c.setWithCost(key, value, int64(cost))
}

type RistrettoCachePartition[K keyable, V any] struct {
	cache   *ristretto.Cache
	Prefix  byte
	Mutable bool
	MaxAge  time.Duration
}

func (c *RistrettoCachePartition[K, V]) setWithCost(key K, value V, cost int64) {
	bkey := fmt.Sprintf("%c%v", c.Prefix, key)
	if !c.Mutable {
		if v, ok := c.cache.Get(bkey); ok && v != nil && !reflect.DeepEqual(v, value) {
			panic(fmt.Sprintf("invalid use of immutable cache tries to change value of %v from %v to %v", key, v, value))
		}
	}
	c.cache.SetWithTTL(bkey, value, cost, c.MaxAge)
}

func (c *RistrettoCachePartition[K, V]) Set(key K, value V) {
	var cost int64
	if cv, ok := any(value).(string); ok {
		cost = int64(len(cv))
	} else {
		cost = int64(unsafe.Sizeof(value))
	}
	c.setWithCost(key, value, cost)
}

func (c *RistrettoCachePartition[K, V]) Unset(key K) {
	bkey := fmt.Sprintf("%c%v", c.Prefix, key)
	if !c.Mutable {
		panic(fmt.Sprintf("invalid use of immutable cache tries to unset value of %v", key))
	}
	c.cache.Del(bkey)
}

func (c *RistrettoCachePartition[K, V]) Get(key K) (value V, ok bool) {
	bkey := fmt.Sprintf("%c%v", c.Prefix, key)
	v, ok := c.cache.Get(bkey)
	if !ok || v == nil {
		var empty V
		return empty, false
	}
	value, ok = v.(V)
	return
}
