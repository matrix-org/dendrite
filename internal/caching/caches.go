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
	RoomServerEvents   Cache[int64, *gomatrixserverlib.Event]
	RoomInfos          Cache[string, types.RoomInfo]
	FederationPDUs     Cache[int64, *gomatrixserverlib.HeaderedEvent]
	FederationEDUs     Cache[int64, *gomatrixserverlib.EDU]
	SpaceSummaryRooms  Cache[string, gomatrixserverlib.MSC2946SpacesResponse]
	LazyLoading        Cache[lazyLoadingCacheKey, string]
}

// Cache is the interface that an implementation must satisfy.
type Cache[K keyable, T any] interface {
	Get(key K) (value T, ok bool)
	Set(key K, value T)
	Unset(key K)
}

type keyable interface {
	// from https://github.com/dgraph-io/ristretto/blob/8e850b710d6df0383c375ec6a7beae4ce48fc8d5/z/z.go#L34
	uint64 | string | []byte | byte | int | int32 | uint32 | int64 | lazyLoadingCacheKey
}

type costable interface {
	CacheCost() int
}
