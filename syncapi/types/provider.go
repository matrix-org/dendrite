package types

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/syncapi/synctypes"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type SyncRequest struct {
	Context       context.Context
	Log           *logrus.Entry
	Device        *userapi.Device
	Response      *Response
	Filter        synctypes.Filter
	Since         StreamingToken
	Timeout       time.Duration
	WantFullState bool

	// Updated by the PDU stream.
	Rooms map[string]string
	// Updated by the PDU stream.
	MembershipChanges map[string]struct{}
	// Updated by the PDU stream.
	IgnoredUsers IgnoredUsers
}

func (r *SyncRequest) IsRoomPresent(roomID string) bool {
	membership, ok := r.Rooms[roomID]
	if !ok {
		return false
	}
	switch membership {
	case spec.Join:
		return true
	case spec.Invite:
		return true
	case spec.Peek:
		return true
	default:
		return false
	}
}
