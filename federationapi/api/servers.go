package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type ServersInRoomProvider interface {
	GetServersForRoom(ctx context.Context, roomID string, event *gomatrixserverlib.Event) []spec.ServerName
}
