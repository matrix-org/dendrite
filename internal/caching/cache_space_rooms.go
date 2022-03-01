package caching

import "github.com/matrix-org/gomatrixserverlib"

const (
	SpaceSummaryRoomsCacheName       = "space_summary_rooms"
	SpaceSummaryRoomsCacheMaxEntries = 100
	SpaceSummaryRoomsCacheMutable    = true
)

type SpaceSummaryRoomsCache interface {
	GetSpaceSummary(roomID string) (r gomatrixserverlib.MSC2946SpacesResponse, ok bool)
	StoreSpaceSummary(roomID string, r gomatrixserverlib.MSC2946SpacesResponse)
}

func (c Caches) GetSpaceSummary(roomID string) (r gomatrixserverlib.MSC2946SpacesResponse, ok bool) {
	val, found := c.SpaceSummaryRooms.Get(roomID)
	if found && val != nil {
		if resp, ok := val.(gomatrixserverlib.MSC2946SpacesResponse); ok {
			return resp, true
		}
	}
	return r, false
}

func (c Caches) StoreSpaceSummary(roomID string, r gomatrixserverlib.MSC2946SpacesResponse) {
	c.SpaceSummaryRooms.Set(roomID, r)
}
