package perform

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
)

type Publisher struct {
	DB storage.Database
}

func (r *Publisher) PerformPublish(
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
