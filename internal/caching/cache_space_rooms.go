package caching

import "github.com/matrix-org/gomatrixserverlib/fclient"

// RoomHierarchy cache caches responses to federated room hierarchy requests (A.K.A. 'space summaries')
type RoomHierarchyCache interface {
	GetRoomHierarchy(roomID string) (r fclient.RoomHierarchyResponse, ok bool)
	StoreRoomHierarchy(roomID string, r fclient.RoomHierarchyResponse)
}

func (c Caches) GetRoomHierarchy(roomID string) (r fclient.RoomHierarchyResponse, ok bool) {
	return c.RoomHierarchies.Get(roomID)
}

func (c Caches) StoreRoomHierarchy(roomID string, r fclient.RoomHierarchyResponse) {
	c.RoomHierarchies.Set(roomID, r)
}
