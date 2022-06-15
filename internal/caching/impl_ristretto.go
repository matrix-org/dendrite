package caching

import (
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/dgraph-io/ristretto"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func MustCreateCache(name string, maxCost CacheSize, enablePrometheus bool) *ristretto.Cache {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e6,
		MaxCost:     int64(maxCost),
		BufferItems: 64,
		Metrics:     true,
	})
	if err != nil {
		panic(err)
	}
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "caching_ristretto",
		Name:      name + "_ratio",
	}, func() float64 {
		return float64(cache.Metrics.Ratio())
	})
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "caching_ristretto",
		Name:      name + "_cost",
	}, func() float64 {
		return float64(cache.Metrics.CostAdded() - cache.Metrics.CostEvicted())
	})
	return cache
}

func NewRistrettoCache(maxCost CacheSize, enablePrometheus bool) (*Caches, error) {

	return &Caches{
		RoomVersions: &RistrettoCachePartition[string, gomatrixserverlib.RoomVersion]{
			cache: MustCreateCache("room_versions", 1*MB, enablePrometheus),
		},
		ServerKeys: &RistrettoCachePartition[string, gomatrixserverlib.PublicKeyLookupResult]{
			cache:   MustCreateCache("server_keys", 32*MB, enablePrometheus),
			Mutable: true,
		},
		RoomServerRoomIDs: &RistrettoCachePartition[int64, string]{
			cache: MustCreateCache("room_ids", 1*MB, enablePrometheus),
		},
		RoomServerEvents: &RistrettoCachePartition[int64, *gomatrixserverlib.Event]{
			cache: MustCreateCache("room_events", 1*GB, enablePrometheus),
		},
		RoomInfos: &RistrettoCachePartition[string, types.RoomInfo]{
			cache:   MustCreateCache("room_infos", 16*MB, enablePrometheus),
			Mutable: true,
			MaxAge:  time.Minute * 5,
		},
		FederationPDUs: &RistrettoCachePartition[int64, *gomatrixserverlib.HeaderedEvent]{
			cache:   MustCreateCache("federation_pdus", 128*MB, enablePrometheus),
			Mutable: true,
		},
		FederationEDUs: &RistrettoCachePartition[int64, *gomatrixserverlib.EDU]{
			cache:   MustCreateCache("federation_edus", 128*MB, enablePrometheus),
			Mutable: true,
		},
		SpaceSummaryRooms: &RistrettoCachePartition[string, gomatrixserverlib.MSC2946SpacesResponse]{
			cache:   MustCreateCache("space_summary_rooms", 128, enablePrometheus), // TODO: not costed
			Mutable: true,
		},
		LazyLoading: &RistrettoCachePartition[string, any]{ // TODO: type
			cache:   MustCreateCache("lazy_loading", 256, enablePrometheus), // TODO: not costed
			Mutable: true,
		},
	}, nil
}

type RistrettoCachePartition[K keyable, V any] struct {
	cache   *ristretto.Cache
	Mutable bool
	MaxAge  time.Duration
}

func (c *RistrettoCachePartition[K, V]) Set(key K, value V) {
	if !c.Mutable {
		if v, ok := c.cache.Get(key); ok && v != nil && !reflect.DeepEqual(v, value) {
			panic(fmt.Sprintf("invalid use of immutable cache tries to change value of %v from %v to %v", key, v, value))
		}
	}
	var cost int64
	if cv, ok := any(value).(costable); ok {
		cost = cv.CacheCost()
	} else if cv, ok := any(value).(string); ok {
		cost = int64(len(cv))
	} else {
		cost = int64(unsafe.Sizeof(value))
	}
	c.cache.SetWithTTL(key, value, cost, c.MaxAge)
}

func (c *RistrettoCachePartition[K, V]) Unset(key K) {
	if !c.Mutable {
		panic(fmt.Sprintf("invalid use of immutable cache tries to unset value of %v", key))
	}
	c.cache.Del(key)
}

func (c *RistrettoCachePartition[K, V]) Get(key K) (value V, ok bool) {
	v, ok := c.cache.Get(key)
	if !ok || v == nil {
		var empty V
		return empty, false
	}
	value, ok = v.(V)
	return
}
