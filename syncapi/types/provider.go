package types

import (
	"context"
	"time"

	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type SyncRequest struct {
	Context       context.Context
	Log           *logrus.Entry
	Device        *userapi.Device
	Response      *Response
	Filter        gomatrixserverlib.Filter
	Since         StreamingToken
	Timeout       time.Duration
	WantFullState bool

	// Updated by the PDU stream.
	Rooms map[string]string
	// Updated by the PDU stream.
	IgnoredUsers IgnoredUsers
}

func (r *SyncRequest) IsRoomPresent(roomID string) bool {
	membership, ok := r.Rooms[roomID]
	if !ok {
		return false
	}
	switch membership {
	case gomatrixserverlib.Join:
		return true
	case gomatrixserverlib.Invite:
		return true
	case gomatrixserverlib.Peek:
		return true
	default:
		return false
	}
}

type StreamProvider interface {
	Setup()

	// Advance will update the latest position of the stream based on
	// an update and will wake callers waiting on StreamNotifyAfter.
	Advance(latest StreamPosition)

	// CompleteSync will update the response to include all updates as needed
	// for a complete sync. It will always return immediately.
	CompleteSync(ctx context.Context, req *SyncRequest) StreamPosition

	// IncrementalSync will update the response to include all updates between
	// the from and to sync positions. It will always return immediately,
	// making no changes if the range contains no updates.
	IncrementalSync(ctx context.Context, req *SyncRequest, from, to StreamPosition) StreamPosition

	// LatestPosition returns the latest stream position for this stream.
	LatestPosition(ctx context.Context) StreamPosition
}
