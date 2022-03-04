package streams

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type NotificationDataStreamProvider struct {
	StreamProvider
}

func (p *NotificationDataStreamProvider) Setup() {
	p.StreamProvider.Setup()

	id, err := p.DB.MaxStreamPositionForNotificationData(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *NotificationDataStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

func (p *NotificationDataStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	// We want counts for all possible rooms, so always start from zero.
	countsByRoom, err := p.DB.GetUserUnreadNotificationCounts(ctx, req.Device.UserID, from, to)
	if err != nil {
		req.Log.WithError(err).Error("GetUserUnreadNotificationCounts failed")
		return from
	}

	// We're merely decorating existing rooms. Note that the Join map
	// values are not pointers.
	for roomID, jr := range req.Response.Rooms.Join {
		counts := countsByRoom[roomID]
		if counts == nil {
			continue
		}

		jr.UnreadNotifications.HighlightCount = counts.UnreadHighlightCount
		jr.UnreadNotifications.NotificationCount = counts.UnreadNotificationCount
		req.Response.Rooms.Join[roomID] = jr
	}
	return to
}
