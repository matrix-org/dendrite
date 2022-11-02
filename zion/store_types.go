/*
Store types
*/
package zion

const (
	ConstSpaceChildEventType  = "m.space.child"
	ConstSpaceParentEventType = "m.space.parent"
)

// Define enum for RoomType
type RoomType int64

const (
	Space RoomType = iota
	Channel
	Unknown
)

func (r RoomType) String() string {
	switch r {
	case Space:
		return "space"
	case Channel:
		return "channel"
	}
	return "unknown"
}

type RoomInfo struct {
	QueryUserId      string // Matrix User ID
	SpaceNetworkId   string
	ChannelNetworkId string
	RoomType         RoomType
	IsOwner          bool
}

type CreatorEvent struct {
	Creator     string `json:"creator"`
	Type        string `json:"type"`
	RoomVersion string `json:"room_version"`
}
