package caching

import (
	"github.com/matrix-org/dendrite/roomserver/types"
)

const (
	RoomServerStateKeyNIDsCacheName       = "roomserver_statekey_nids"
	RoomServerStateKeyNIDsCacheMaxEntries = 1024
	RoomServerStateKeyNIDsCacheMutable    = false

	RoomServerEventTypeNIDsCacheName       = "roomserver_eventtype_nids"
	RoomServerEventTypeNIDsCacheMaxEntries = 64
	RoomServerEventTypeNIDsCacheMutable    = false

	RoomServerRoomNIDsCacheName       = "roomserver_room_nids"
	RoomServerRoomNIDsCacheMaxEntries = 1024
	RoomServerRoomNIDsCacheMutable    = false
)

type RoomServerCaches interface {
	RoomServerNIDsCache
	RoomVersionCache
}

// RoomServerNIDsCache contains the subset of functions needed for
// a roomserver NID cache.
type RoomServerNIDsCache interface {
	GetRoomServerStateKeyNID(stateKey string) (types.EventStateKeyNID, bool)
	StoreRoomServerStateKeyNID(stateKey string, nid types.EventStateKeyNID)

	GetRoomServerEventTypeNID(eventType string) (types.EventTypeNID, bool)
	StoreRoomServerEventTypeNID(eventType string, nid types.EventTypeNID)

	GetRoomServerRoomNID(roomID string) (types.RoomNID, bool)
	StoreRoomServerRoomNID(roomID string, nid types.RoomNID)
}

func (c Caches) GetRoomServerStateKeyNID(stateKey string) (types.EventStateKeyNID, bool) {
	val, found := c.RoomServerStateKeyNIDs.Get(stateKey)
	if found && val != nil {
		if stateKeyNID, ok := val.(types.EventStateKeyNID); ok {
			return stateKeyNID, true
		}
	}
	return 0, false
}

func (c Caches) StoreRoomServerStateKeyNID(stateKey string, nid types.EventStateKeyNID) {
	c.RoomVersions.Set(stateKey, nid)
}

func (c Caches) GetRoomServerEventTypeNID(eventType string) (types.EventTypeNID, bool) {
	val, found := c.RoomServerEventTypeNIDs.Get(eventType)
	if found && val != nil {
		if eventTypeNID, ok := val.(types.EventTypeNID); ok {
			return eventTypeNID, true
		}
	}
	return 0, false
}

func (c Caches) StoreRoomServerEventTypeNID(eventType string, nid types.EventTypeNID) {
	c.RoomVersions.Set(eventType, nid)
}

func (c Caches) GetRoomServerRoomNID(roomID string) (types.RoomNID, bool) {
	val, found := c.RoomServerRoomNIDs.Get(roomID)
	if found && val != nil {
		if roomNID, ok := val.(types.RoomNID); ok {
			return roomNID, true
		}
	}
	return 0, false
}

func (c Caches) StoreRoomServerRoomNID(roomID string, nid types.RoomNID) {
	c.RoomVersions.Set(roomID, nid)
}
