package types

import (
	"context"

	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type StreamProvider interface {
	StreamSetup()
	StreamAdvance(latest StreamPosition)

	// Range will update the response to include all updates between
	// the from and to sync positions. It will always return immediately,
	// making no changes if the range contains no updates.
	StreamRange(ctx context.Context, res *Response, device *userapi.Device, from, to StreamingToken, filter gomatrixserverlib.EventFilter) StreamPosition

	// NotifyAfter returns a channel which will be closed once the
	// stream advances past the "from" position.
	StreamNotifyAfter(ctx context.Context, from StreamingToken) chan struct{}

	// LatestPosition returns the latest stream position for this stream.
	StreamLatestPosition(ctx context.Context) StreamPosition
}

type TopologyProvider interface {
	// Range will update the response to include all updates between
	// the from and to sync positions for the given room. It will always
	// return immediately, making no changes if the range contains no
	// updates.
	TopologyRange(ctx context.Context, res *Response, roomID string, from, to TopologyToken, filter gomatrixserverlib.EventFilter)

	// LatestPosition returns the latest stream position for this stream
	// for the given room.
	TopologyLatestPosition(ctx context.Context, roomID string) StreamPosition
}
