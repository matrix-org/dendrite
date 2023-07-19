// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package caching

import (
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/dgraph-io/ristretto"
	"github.com/dgraph-io/ristretto/z"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
)

const (
	roomVersionsCache byte = iota + 1
	serverKeysCache
	roomNIDsCache
	roomIDsCache
	roomEventsCache
	federationPDUsCache
	federationEDUsCache
	spaceSummaryRoomsCache
	lazyLoadingCache
	eventStateKeyCache
	eventTypeCache
	eventTypeNIDCache
	eventStateKeyNIDCache
)

const (
	DisableMetrics = false
	EnableMetrics  = true
)

func NewRistrettoCache(maxCost config.DataUnit, maxAge time.Duration, enablePrometheus bool) *Caches {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64((maxCost / 1024) * 10), // 10 counters per 1KB data, affects bloom filter size
		BufferItems: 64,                           // recommended by the ristretto godocs as a sane buffer size value
		MaxCost:     int64(maxCost),               // max cost is in bytes, as per the Dendrite config
		Metrics:     true,
		KeyToHash: func(key interface{}) (uint64, uint64) {
			return z.KeyToHash(key)
		},
	})
	if err != nil {
		panic(err)
	}
	if enablePrometheus {
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
	}
	return &Caches{
		RoomVersions: &RistrettoCachePartition[string, gomatrixserverlib.RoomVersion]{ // room ID -> room version
			cache:  cache,
			Prefix: roomVersionsCache,
			MaxAge: maxAge,
		},
		ServerKeys: &RistrettoCachePartition[string, gomatrixserverlib.PublicKeyLookupResult]{ // server name -> server keys
			cache:   cache,
			Prefix:  serverKeysCache,
			Mutable: true,
			MaxAge:  maxAge,
		},
		RoomServerRoomNIDs: &RistrettoCachePartition[string, types.RoomNID]{ // room ID -> room NID
			cache:  cache,
			Prefix: roomNIDsCache,
			MaxAge: maxAge,
		},
		RoomServerRoomIDs: &RistrettoCachePartition[types.RoomNID, string]{ // room NID -> room ID
			cache:  cache,
			Prefix: roomIDsCache,
			MaxAge: maxAge,
		},
		RoomServerEvents: &RistrettoCostedCachePartition[int64, *types.HeaderedEvent]{ // event NID -> event
			&RistrettoCachePartition[int64, *types.HeaderedEvent]{
				cache:   cache,
				Prefix:  roomEventsCache,
				MaxAge:  maxAge,
				Mutable: true,
			},
		},
		RoomServerStateKeys: &RistrettoCachePartition[types.EventStateKeyNID, string]{ // event NID -> event state key
			cache:  cache,
			Prefix: eventStateKeyCache,
			MaxAge: maxAge,
		},
		RoomServerStateKeyNIDs: &RistrettoCachePartition[string, types.EventStateKeyNID]{ // eventStateKey -> eventStateKey NID
			cache:  cache,
			Prefix: eventStateKeyNIDCache,
			MaxAge: maxAge,
		},
		RoomServerEventTypeNIDs: &RistrettoCachePartition[string, types.EventTypeNID]{ // eventType -> eventType NID
			cache:  cache,
			Prefix: eventTypeCache,
			MaxAge: maxAge,
		},
		RoomServerEventTypes: &RistrettoCachePartition[types.EventTypeNID, string]{ // eventType NID -> eventType
			cache:  cache,
			Prefix: eventTypeNIDCache,
			MaxAge: maxAge,
		},
		FederationPDUs: &RistrettoCostedCachePartition[int64, *types.HeaderedEvent]{ // queue NID -> PDU
			&RistrettoCachePartition[int64, *types.HeaderedEvent]{
				cache:   cache,
				Prefix:  federationPDUsCache,
				Mutable: true,
				MaxAge:  lesserOf(time.Hour/2, maxAge),
			},
		},
		FederationEDUs: &RistrettoCostedCachePartition[int64, *gomatrixserverlib.EDU]{ // queue NID -> EDU
			&RistrettoCachePartition[int64, *gomatrixserverlib.EDU]{
				cache:   cache,
				Prefix:  federationEDUsCache,
				Mutable: true,
				MaxAge:  lesserOf(time.Hour/2, maxAge),
			},
		},
		RoomHierarchies: &RistrettoCachePartition[string, fclient.RoomHierarchyResponse]{ // room ID -> space response
			cache:   cache,
			Prefix:  spaceSummaryRoomsCache,
			Mutable: true,
			MaxAge:  maxAge,
		},
		LazyLoading: &RistrettoCachePartition[lazyLoadingCacheKey, string]{ // composite key -> event ID
			cache:   cache,
			Prefix:  lazyLoadingCache,
			Mutable: true,
			MaxAge:  maxAge,
		},
	}
}

type RistrettoCostedCachePartition[k keyable, v costable] struct {
	*RistrettoCachePartition[k, v]
}

func (c *RistrettoCostedCachePartition[K, V]) Set(key K, value V) {
	cost := value.CacheCost()
	c.setWithCost(key, value, int64(cost))
}

type RistrettoCachePartition[K keyable, V any] struct {
	cache   *ristretto.Cache //nolint:all,unused
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
	c.cache.SetWithTTL(bkey, value, int64(len(bkey))+cost, c.MaxAge)
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

func lesserOf(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
