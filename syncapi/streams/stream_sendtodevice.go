package streams

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type SendToDeviceStreamProvider struct {
	DefaultStreamProvider
}

func (p *SendToDeviceStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseTransaction,
) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := snapshot.MaxStreamPositionForSendToDeviceMessages(ctx)
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *SendToDeviceStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.LatestPosition(ctx))
}

func (p *SendToDeviceStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	// See if we have any new tasks to do for the send-to-device messaging.
	lastPos, events, err := snapshot.SendToDeviceUpdatesForSync(req.Context, req.Device.UserID, req.Device.ID, from, to)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.SendToDeviceUpdatesForSync failed")
		return from
	}

	// Add the updates into the sync response.
	for _, event := range events {
		// skip ignored user events
		if _, ok := req.IgnoredUsers.List[event.Sender]; ok {
			continue
		}
		req.Response.ToDevice.Events = append(req.Response.ToDevice.Events, event.SendToDeviceEvent)
	}

	return lastPos
}
