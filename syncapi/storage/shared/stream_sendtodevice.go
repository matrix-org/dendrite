package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type SendToDeviceStreamProvider struct {
	StreamProvider
}

func (p *SendToDeviceStreamProvider) Range(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {

	return to
}
