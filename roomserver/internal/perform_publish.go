package internal

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
)

func (r *RoomserverInternalAPI) PerformPublish(
	ctx context.Context,
	req *api.PerformPublishRequest,
	res *api.PerformPublishResponse,
) {
	err := r.DB.PublishRoom(ctx, req.RoomID, req.Visibility == "public")
	if err != nil {
		res.Error = &api.PerformError{
			Msg: err.Error(),
		}
	}
}
