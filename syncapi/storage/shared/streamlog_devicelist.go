package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type DeviceListStreamProvider struct {
	StreamLogProvider
}

func (p *DeviceListStreamProvider) Range(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.LogPosition,
) types.LogPosition {

	return to
}
