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
	Filter        gomatrixserverlib.EventFilter
	Since         StreamingToken
	Limit         int
	Timeout       time.Duration
	WantFullState bool

	// Updated by the PDU stream.
	Rooms map[string]string
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

	// NotifyAfter returns a channel which will be closed once the
	// stream advances past the "from" position.
	NotifyAfter(ctx context.Context, from StreamPosition) chan struct{}

	// LatestPosition returns the latest stream position for this stream.
	LatestPosition(ctx context.Context) StreamPosition
}

type StreamLogProvider interface {
	Setup()
	Advance(latest LogPosition)
	CompleteSync(ctx context.Context, req *SyncRequest) LogPosition
	IncrementalSync(ctx context.Context, req *SyncRequest, from, to LogPosition) LogPosition
	NotifyAfter(ctx context.Context, from LogPosition) chan struct{}
	LatestPosition(ctx context.Context) LogPosition
}

type TopologyProvider interface {
	// Range will update the response to include all updates between
	// the from and to sync positions for the given room. It will always
	// return immediately, making no changes if the range contains no
	// updates.
	TopologyRange(ctx context.Context, res *Response, roomID string, from, to TopologyToken, filter gomatrixserverlib.EventFilter)

	// LatestPosition returns the latest stream position for this stream
	// for the given room.
	TopologyLatestPosition(ctx context.Context, roomID string) TopologyToken
}
