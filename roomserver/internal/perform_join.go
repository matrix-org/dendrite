package internal

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
)

// WriteOutputEvents implements OutputRoomEventWriter
func (r *RoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) error {
	return nil
}
