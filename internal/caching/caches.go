package caching

// Caches contains a set of references to caches. They may be
// different implementations as long as they satisfy the Cache
// interface.
type Caches struct {
	RoomVersions            Cache // RoomVersionCache
	ServerKeys              Cache // ServerKeyCache
	RoomServerStateKeyNIDs  Cache // RoomServerNIDsCache
	RoomServerEventTypeNIDs Cache // RoomServerNIDsCache
	RoomServerRoomNIDs      Cache // RoomServerNIDsCache
	RoomServerRoomIDs       Cache // RoomServerNIDsCache
}

// Cache is the interface that an implementation must satisfy.
type Cache interface {
	Get(key string) (value interface{}, ok bool)
	Set(key string, value interface{})
	Unset(key string)
}
