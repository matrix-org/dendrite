package storage

import (
	"context"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/publicroomsapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	common.PartitionStorer
	GetRoomVisibility(ctx context.Context, roomID string) (bool, error)
	SetRoomVisibility(ctx context.Context, visible bool, roomID string) error
	CountPublicRooms(ctx context.Context) (int64, error)
	GetPublicRooms(ctx context.Context, offset int64, limit int16, filter string) ([]types.PublicRoom, error)
	UpdateRoomFromEvents(ctx context.Context, eventsToAdd []gomatrixserverlib.Event, eventsToRemove []gomatrixserverlib.Event) error
	UpdateRoomFromEvent(ctx context.Context, event gomatrixserverlib.Event) error
}
