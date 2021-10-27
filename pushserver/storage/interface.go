package storage

import (
	"context"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
)

type Database interface {
	internal.PartitionStorer
	CreatePusher(ctx context.Context, pusher api.Pusher, localpart string) error
	GetPushers(ctx context.Context, localpart string) ([]api.Pusher, error)
	RemovePusher(ctx context.Context, appId, pushkey, localpart string) error
	RemovePushers(ctx context.Context, appId, pushkey string) error

	InsertNotification(ctx context.Context, localpart, eventID string, tweaks map[string]interface{}, n *api.Notification) error
	DeleteNotificationsUpTo(ctx context.Context, localpart, roomID, upToEventID string) (affected bool, _ error)
	SetNotificationsRead(ctx context.Context, localpart, roomID, upToEventID string, b bool) (affected bool, _ error)
	GetNotifications(ctx context.Context, localpart string, fromID int64, limit int, filter NotificationFilter) ([]*api.Notification, int64, error)
	GetNotificationCount(ctx context.Context, localpart string, filter NotificationFilter) (int64, error)
	GetRoomNotificationCounts(ctx context.Context, localpart, roomID string) (total int64, highlight int64, _ error)
}

type NotificationFilter = tables.NotificationFilter
