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

	RoomServerRoomIDsCacheName       = "roomserver_room_ids"
	RoomServerRoomIDsCacheMaxEntries = 1024
	RoomServerRoomIDsCacheMutable    = false
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

	GetRoomServerRoomID(roomNID types.RoomNID) (string, bool)
	StoreRoomServerRoomID(roomNID types.RoomNID, roomID string)
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
	c.RoomServerStateKeyNIDs.Set(stateKey, nid)
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
	c.RoomServerEventTypeNIDs.Set(eventType, nid)
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

func (c Caches) StoreRoomServerRoomNID(roomID string, roomNID types.RoomNID) {
	c.RoomServerRoomNIDs.Set(roomID, roomNID)
	c.RoomServerRoomIDs.Set(string(roomNID), roomID)
}

func (c Caches) GetRoomServerRoomID(roomNID types.RoomNID) (string, bool) {
	val, found := c.RoomServerRoomIDs.Get(string(roomNID))
	if found && val != nil {
		if roomID, ok := val.(string); ok {
			return roomID, true
		}
	}
	return "", false
}

func (c Caches) StoreRoomServerRoomID(roomNID types.RoomNID, roomID string) {
	c.StoreRoomServerRoomNID(roomID, roomNID)
}
