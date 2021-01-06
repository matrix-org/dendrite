package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type AccountDataStreamProvider struct {
	StreamProvider
}

func (p *AccountDataStreamProvider) Setup() {
	p.StreamProvider.Setup()
}

func (p *AccountDataStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

func (p *AccountDataStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {

	return to
}
