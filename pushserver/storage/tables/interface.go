package tables

import (
	"context"

	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type Pusher interface {
	InsertPusher(
		ctx context.Context, session_id int64,
		pushkey string, pushkeyTS gomatrixserverlib.Timestamp, kind api.PusherKind,
		appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
	) error
	SelectPushers(
		ctx context.Context, localpart string,
	) ([]api.Pusher, error)
	DeletePusher(
		ctx context.Context, appid, pushkey, localpart string,
	) error
	DeletePushers(
		ctx context.Context, appid, pushkey string,
	) error
}

type Notifications interface {
	Insert(ctx context.Context, localpart, eventID string, highlight bool, n *api.Notification) error
	DeleteUpTo(ctx context.Context, localpart, roomID, eventID string) (affected bool, _ error)
	UpdateRead(ctx context.Context, localpart, roomID, eventID string, v bool) (affected bool, _ error)
	Select(ctx context.Context, localpart string, fromID int64, limit int, filter NotificationFilter) ([]*api.Notification, int64, error)
	SelectCount(ctx context.Context, localpart string, filter NotificationFilter) (int64, error)
	SelectRoomCounts(ctx context.Context, localpart, roomID string) (total int64, highlight int64, _ error)
}

type NotificationFilter uint32

const (
	// HighlightNotifications returns notifications that had a
	// "highlight" tweak assigned to them from evaluating push rules.
	HighlightNotifications NotificationFilter = 1 << iota

	// NonHighlightNotifications returns notifications that don't
	// match HighlightNotifications.
	NonHighlightNotifications

	// NoNotifications is a filter to exclude all types of
	// notifications. It's useful as a zero value, but isn't likely to
	// be used in a call to Notifications.Select*.
	NoNotifications NotificationFilter = 0

	// AllNotifications is a filter to include all types of
	// notifications in Notifications.Select*. Note that PostgreSQL
	// balks if this doesn't fit in INTEGER, even though we use
	// uint32.
	AllNotifications NotificationFilter = (1 << 31) - 1
)
