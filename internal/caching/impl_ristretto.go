package caching

import (
	"fmt"
	"time"

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
		RoomServerRoomIDs: &RistrettoCachePartition[types.RoomNID, string]{
			cache: cache,
			Name:  "room_ids",
		},
		RoomInfos: &RistrettoCachePartition[string, types.RoomInfo]{
			cache:   cache,
			Name:    "room_infos",
			Mutable: true,
			MaxAge:  time.Minute * 5,
		},
		FederationPDUs: &RistrettoCachePartition[int64, *gomatrixserverlib.HeaderedEvent]{
			cache: cache,
			Name:  "federation_events_pdu",
		},
		FederationEDUs: &RistrettoCachePartition[int64, *gomatrixserverlib.EDU]{
			cache: cache,
			Name:  "federation_events_edu",
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
	if !c.Mutable {
		if _, ok := c.cache.Get(key); ok {
			panic(fmt.Sprintf("invalid use of immutable cache tries to replace value of %v", key))
		}
	}
	cost := int64(1)
	if cv, ok := any(value).(costable); ok {
		cost = int64(cv.CacheCost())
	}
	c.cache.SetWithTTL(key, value, cost, c.MaxAge)
}

func (c *RistrettoCachePartition[K, V]) Unset(key K) {
	if c.cache == nil {
		return
	}
	if !c.Mutable {
		panic(fmt.Sprintf("invalid use of immutable cache tries to unset value of %v", key))
	}
	c.cache.Del(key)
}

func (c *RistrettoCachePartition[K, V]) Get(key K) (value V, ok bool) {
	if c.cache == nil {
		var empty V
		return empty, false
	}
	v, ok := c.cache.Get(key)
	value, ok = v.(V)
	return
}
