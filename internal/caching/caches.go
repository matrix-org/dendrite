package caching

import (
	"time"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// Caches contains a set of references to caches. They may be
// different implementations as long as they satisfy the Cache
// interface.
type Caches struct {
	RoomVersions       Cache[string, gomatrixserverlib.RoomVersion]
	ServerKeys         Cache[string, gomatrixserverlib.PublicKeyLookupResult]
	RoomServerRoomNIDs Cache[string, types.RoomNID]
	RoomServerRoomIDs  Cache[int64, string]
	RoomInfos          Cache[string, types.RoomInfo]
	FederationPDUs     Cache[int64, *gomatrixserverlib.HeaderedEvent]
	FederationEDUs     Cache[int64, *gomatrixserverlib.EDU]
	SpaceSummaryRooms  Cache[string, gomatrixserverlib.MSC2946SpacesResponse]
	LazyLoading        Cache[string, any]
}

// Cache is the interface that an implementation must satisfy.
type Cache[K keyable, T any] interface {
	Get(key K) (value T, ok bool)
	Set(key K, value T)
	Unset(key K)
}

type keyable interface {
	// from https://github.com/dgraph-io/ristretto/blob/8e850b710d6df0383c375ec6a7beae4ce48fc8d5/z/z.go#L34
	uint64 | string | []byte | byte | int | int32 | uint32 | int64
}

type costable interface {
	CacheCost() int64
}

type CacheSize int64

const (
	_            = iota
	KB CacheSize = 1 << (10 * iota)
	MB
	GB
	TB
)

const CacheNoMaxAge = time.Duration(0)
