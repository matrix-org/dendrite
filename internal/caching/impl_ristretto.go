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

func NewRistrettoCache(maxCost CacheSize, enablePrometheus bool) (*Caches, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     int64(maxCost),
		BufferItems: 64,
		Metrics:     true,
	})
	if err != nil {
		return nil, err
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
	return &Caches{
		RoomVersions: &RistrettoCachePartition[string, gomatrixserverlib.RoomVersion]{
			cache: cache,
			Name:  "room_versions",
		},
		ServerKeys: &RistrettoCachePartition[string, gomatrixserverlib.PublicKeyLookupResult]{
			cache:   cache,
			Name:    "server_keys",
			Mutable: true,
		},
		RoomServerRoomIDs: &RistrettoCachePartition[int64, string]{
			cache: cache,
			Name:  "room_ids",
		},
		RoomServerEvents: &RistrettoCachePartition[int64, *gomatrixserverlib.Event]{
			cache: cache,
			Name:  "room_events",
		},
		RoomInfos: &RistrettoCachePartition[string, types.RoomInfo]{
			cache:   cache,
			Name:    "room_infos",
			Mutable: true,
			MaxAge:  time.Minute * 5,
		},
		FederationPDUs: &RistrettoCachePartition[int64, *gomatrixserverlib.HeaderedEvent]{
			cache:   cache,
			Name:    "federation_events_pdu",
			Mutable: true,
		},
		FederationEDUs: &RistrettoCachePartition[int64, *gomatrixserverlib.EDU]{
			cache:   cache,
			Name:    "federation_events_edu",
			Mutable: true,
		},
		SpaceSummaryRooms: &RistrettoCachePartition[string, gomatrixserverlib.MSC2946SpacesResponse]{
			cache:   cache,
			Name:    "space_summary_rooms",
			Mutable: true,
		},
		LazyLoading: &RistrettoCachePartition[string, any]{ // TODO: type
			cache:   cache,
			Name:    "lazy_loading",
			Mutable: true,
		},
	}, nil
}

type RistrettoCachePartition[K keyable, V any] struct {
	cache   *ristretto.Cache
	Name    string
	Mutable bool
	MaxAge  time.Duration
}

func (c *RistrettoCachePartition[K, V]) Set(key K, value V) {
	strkey := fmt.Sprintf("%v\000%s", key, c.Name)
	if !c.Mutable {
		if v, ok := c.cache.Get(strkey); ok && v != nil && !reflect.DeepEqual(v, value) {
			panic(fmt.Sprintf("invalid use of immutable cache tries to change value of %v from %v to %v", strkey, v, value))
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
	c.cache.SetWithTTL(strkey, value, cost, c.MaxAge)
}

func (c *RistrettoCachePartition[K, V]) Unset(key K) {
	strkey := fmt.Sprintf("%s_%v", c.Name, key)
	if !c.Mutable {
		panic(fmt.Sprintf("invalid use of immutable cache tries to unset value of %v", strkey))
	}
	c.cache.Del(strkey)
}

func (c *RistrettoCachePartition[K, V]) Get(key K) (value V, ok bool) {
	strkey := fmt.Sprintf("%s_%v", c.Name, key)
	v, ok := c.cache.Get(strkey)
	if !ok || v == nil {
		var empty V
		return empty, false
	}
	value, ok = v.(V)
	return
}
