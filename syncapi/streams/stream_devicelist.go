package streams

import (
	"context"

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type DeviceListStreamProvider struct {
	DefaultStreamProvider
	rsAPI  api.SyncRoomserverAPI
	keyAPI keyapi.SyncKeyAPI
}

func (p *DeviceListStreamProvider) latestPosition(
	ctx context.Context, snapshot storage.DatabaseSnapshot,
) types.StreamPosition {
	return 0 // TODO: is this the right thing to do?
}

func (p *DeviceListStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseSnapshot,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.latestPosition(ctx, snapshot)
}

func (p *DeviceListStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseSnapshot,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	var err error
	to, _, err = internal.DeviceListCatchup(context.Background(), snapshot, p.keyAPI, p.rsAPI, req.Device.UserID, req.Response, from, to)
	if err != nil {
		req.Log.WithError(err).Error("internal.DeviceListCatchup failed")
		return from
	}
	err = internal.DeviceOTKCounts(req.Context, p.keyAPI, req.Device.UserID, req.Device.ID, req.Response)
	if err != nil {
		req.Log.WithError(err).Error("internal.DeviceListCatchup failed")
		return from
	}

	return to
}
