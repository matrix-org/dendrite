package streams

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"

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
	from, _ types.StreamPosition,
) types.StreamPosition {
	to := p.LatestPosition(ctx)
	countsByRoom, err := p.DB.GetUserUnreadNotificationCounts(ctx, req.Device.UserID, from, to)
	if err != nil {
		req.Log.WithError(err).Error("GetUserUnreadNotificationCounts failed")
		return from
	}

	// Add notification data based on the membership of the user
	for roomID, counts := range countsByRoom {
		if req.Rooms[roomID] != gomatrixserverlib.Join {
			continue
		}
		// If there's no join response, add a new one
		jr, ok := req.Response.Rooms.Join[roomID]
		if !ok {
			jr = *types.NewJoinResponse()
		}

		jr.UnreadNotifications = &types.UnreadNotifications{
			HighlightCount:    counts.UnreadHighlightCount,
			NotificationCount: counts.UnreadNotificationCount,
		}
		req.Response.Rooms.Join[roomID] = jr
	}
	return to
}
