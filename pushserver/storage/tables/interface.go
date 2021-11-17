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
