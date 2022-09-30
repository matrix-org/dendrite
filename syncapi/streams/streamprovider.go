package streams

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type StreamProvider interface {
	Setup(ctx context.Context, snapshot storage.DatabaseTransaction)

	// Advance will update the latest position of the stream based on
	// an update and will wake callers waiting on StreamNotifyAfter.
	Advance(latest types.StreamPosition)

	// CompleteSync will update the response to include all updates as needed
	// for a complete sync. It will always return immediately.
	CompleteSync(ctx context.Context, snapshot storage.DatabaseTransaction, req *types.SyncRequest) types.StreamPosition

	// IncrementalSync will update the response to include all updates between
	// the from and to sync positions. It will always return immediately,
	// making no changes if the range contains no updates.
	IncrementalSync(ctx context.Context, snapshot storage.DatabaseTransaction, req *types.SyncRequest, from, to types.StreamPosition) types.StreamPosition

	// LatestPosition returns the latest stream position for this stream.
	LatestPosition(ctx context.Context) types.StreamPosition
}
