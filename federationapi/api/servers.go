package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
)

type ServersInRoomProvider interface {
	GetServersForRoom(ctx context.Context, roomID string, event *gomatrixserverlib.Event) []gomatrixserverlib.ServerName
}
