package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type DeviceListStreamProvider struct {
	StreamLogProvider
}

func (p *DeviceListStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.LogPosition {
	return p.LatestPosition(ctx)
}

func (p *DeviceListStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.LogPosition,
) types.LogPosition {

	return to
}
