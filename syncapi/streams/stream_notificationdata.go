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

	// Add notification data to rooms.
	// Create an empty JoinResponse if the room isn't in this response
	for roomID, notificationData := range countsByRoom {
		jr, exists := req.Response.Rooms.Join[roomID]
		if !exists {
			jr = *types.NewJoinResponse()
		}
		jr.UnreadNotifications = &types.UnreadNotifications{
			HighlightCount:    notificationData.UnreadHighlightCount,
			NotificationCount: notificationData.UnreadNotificationCount,
		}
		req.Response.Rooms.Join[roomID] = jr
	}
	return to
}
