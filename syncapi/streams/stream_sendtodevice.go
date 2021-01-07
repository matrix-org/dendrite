package streams

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type SendToDeviceStreamProvider struct {
	StreamProvider
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
	lastPos, events, updates, deletions, err := p.DB.SendToDeviceUpdatesForSync(req.Context, req.Device.UserID, req.Device.ID, req.Since)
	if err != nil {
		return to // nil, fmt.Errorf("rp.db.SendToDeviceUpdatesForSync: %w", err)
	}

	// Before we return the sync response, make sure that we take action on
	// any send-to-device database updates or deletions that we need to do.
	// Then add the updates into the sync response.
	if len(updates) > 0 || len(deletions) > 0 {
		// Handle the updates and deletions in the database.
		err = p.DB.CleanSendToDeviceUpdates(context.Background(), updates, deletions, req.Since)
		if err != nil {
			return to // res, fmt.Errorf("rp.db.CleanSendToDeviceUpdates: %w", err)
		}
	}
	if len(events) > 0 {
		// Add the updates into the sync response.
		for _, event := range events {
			req.Response.ToDevice.Events = append(req.Response.ToDevice.Events, event.SendToDeviceEvent)
		}
	}

	return lastPos
}
