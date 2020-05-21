package caching

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type ImmutableInMemoryLRUCache struct {
	roomVersions *lru.Cache
	serverKeys   *lru.Cache
}

func NewImmutableInMemoryLRUCache() (*ImmutableInMemoryLRUCache, error) {
	roomVersionCache, rvErr := lru.New(RoomVersionMaxCacheEntries)
	if rvErr != nil {
		return nil, rvErr
	}
	serverKeysCache, rvErr := lru.New(ServerKeysMaxCacheEntries)
	if rvErr != nil {
		return nil, rvErr
	}
	cache := &ImmutableInMemoryLRUCache{
		roomVersions: roomVersionCache,
		serverKeys:   serverKeysCache,
	}
	cache.configureMetrics()
	return cache, nil
}

func (c *ImmutableInMemoryLRUCache) configureMetrics() {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "caching",
		Name:      "number_room_version_entries",
		Help:      "The number of room version entries cached.",
	}, func() float64 {
		return float64(c.roomVersions.Len())
	})

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "caching",
		Name:      "number_server_key_entries",
		Help:      "The number of server key entries cached.",
	}, func() float64 {
		return float64(c.serverKeys.Len())
	})
}

func checkForInvalidMutation(cache *lru.Cache, key string, value interface{}) {
	if peek, ok := cache.Peek(key); ok && peek != value {
		panic(fmt.Sprintf("invalid use of immutable cache tries to mutate existing value of %q", key))
	}
}

func (c *ImmutableInMemoryLRUCache) GetRoomVersion(roomID string) (gomatrixserverlib.RoomVersion, bool) {
	val, found := c.roomVersions.Get(roomID)
	if found && val != nil {
		if roomVersion, ok := val.(gomatrixserverlib.RoomVersion); ok {
			return roomVersion, true
		}
	}
	return "", false
}

func (c *ImmutableInMemoryLRUCache) StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion) {
	checkForInvalidMutation(c.roomVersions, roomID, roomVersion)
	c.roomVersions.Add(roomID, roomVersion)
}

func (c *ImmutableInMemoryLRUCache) GetServerKey(
	request gomatrixserverlib.PublicKeyLookupRequest,
) (gomatrixserverlib.PublicKeyLookupResult, bool) {
	key := fmt.Sprintf("%s/%s", request.ServerName, request.KeyID)
	val, found := c.serverKeys.Get(key)
	if found && val != nil {
		if keyLookupResult, ok := val.(gomatrixserverlib.PublicKeyLookupResult); ok {
			return keyLookupResult, true
		}
	}
	return gomatrixserverlib.PublicKeyLookupResult{}, false
}

func (c *ImmutableInMemoryLRUCache) StoreServerKey(
	request gomatrixserverlib.PublicKeyLookupRequest,
	response gomatrixserverlib.PublicKeyLookupResult,
) {
	key := fmt.Sprintf("%s/%s", request.ServerName, request.KeyID)
	checkForInvalidMutation(c.roomVersions, key, response)
	c.serverKeys.Add(request, response)
}
