package streams

import (
	"context"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type NotificationDataStreamProvider struct {
	DefaultStreamProvider
}

func (p *NotificationDataStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseTransaction,
) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := snapshot.MaxStreamPositionForNotificationData(ctx)
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *NotificationDataStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.LatestPosition(ctx))
}

func (p *NotificationDataStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
	from, _ types.StreamPosition,
) types.StreamPosition {
	// Get the unread notifications for rooms in our join response.
	// This is to ensure clients always have an unread notification section
	// and can display the correct numbers.
	countsByRoom, err := snapshot.GetUserUnreadNotificationCountsForRooms(ctx, req.Device.UserID, req.Rooms)
	if err != nil {
		req.Log.WithError(err).Error("GetUserUnreadNotificationCountsForRooms failed")
		return from
	}

	// We're merely decorating existing rooms.
	for roomID, jr := range req.Response.Rooms.Join {
		counts := countsByRoom[roomID]
		if counts == nil {
			counts = &eventutil.NotificationData{}
		}
		jr.UnreadNotifications = &types.UnreadNotifications{
			HighlightCount:    counts.UnreadHighlightCount,
			NotificationCount: counts.UnreadNotificationCount,
		}
		req.Response.Rooms.Join[roomID] = jr
	}

	return p.LatestPosition(ctx)
}
