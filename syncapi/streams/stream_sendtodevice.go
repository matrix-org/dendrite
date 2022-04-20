package streams

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type SendToDeviceStreamProvider struct {
	StreamProvider
}

func (p *SendToDeviceStreamProvider) Setup() {
	p.StreamProvider.Setup()

	id, err := p.DB.MaxStreamPositionForSendToDeviceMessages(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *SendToDeviceStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

func (p *SendToDeviceStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	// See if we have any new tasks to do for the send-to-device messaging.
	lastPos, events, err := p.DB.SendToDeviceUpdatesForSync(req.Context, req.Device.UserID, req.Device.ID, from, to)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.SendToDeviceUpdatesForSync failed")
		return from
	}

	if len(events) > 0 {
		// Clean up old send-to-device messages from before this stream position.
		if err := p.DB.CleanSendToDeviceUpdates(req.Context, req.Device.UserID, req.Device.ID, from); err != nil {
			req.Log.WithError(err).Error("p.DB.CleanSendToDeviceUpdates failed")
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
	}

	return lastPos
}
