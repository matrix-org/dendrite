package storage

import (
	"context"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/pushserver/api"
)

type Database interface {
	internal.PartitionStorer
	CreatePusher(ctx context.Context, pusher api.Pusher, localpart string) error
	GetPushers(ctx context.Context, localpart string) ([]api.Pusher, error)
	RemovePusher(ctx context.Context, appId, pushkey, localpart string) error
	RemovePushers(ctx context.Context, appId, pushkey string) error
}
