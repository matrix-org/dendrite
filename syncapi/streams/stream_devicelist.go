package streams

import (
	"context"

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type DeviceListStreamProvider struct {
	PartitionedStreamProvider
	rsAPI  api.RoomserverInternalAPI
	keyAPI keyapi.KeyInternalAPI
}

func (p *DeviceListStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.LogPosition {
	return p.IncrementalSync(ctx, req, types.LogPosition{}, p.LatestPosition(ctx))
}

func (p *DeviceListStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.LogPosition,
) types.LogPosition {
	var err error
	to, _, err = internal.DeviceListCatchup(context.Background(), p.keyAPI, p.rsAPI, req.Device.UserID, req.Response, from, to)
	if err != nil {
		return to // nil, fmt.Errorf("internal.DeviceListCatchup: %w", err)
	}
	err = internal.DeviceOTKCounts(req.Context, p.keyAPI, req.Device.UserID, req.Device.ID, req.Response)
	if err != nil {
		return to // res, fmt.Errorf("internal.DeviceOTKCounts: %w", err)
	}

	return to
}
