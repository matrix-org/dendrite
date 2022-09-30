package streams

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type DefaultStreamProvider struct {
	DB          storage.Database
	latest      types.StreamPosition
	latestMutex sync.RWMutex
}

func (p *DefaultStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseTransaction,
) {
}

func (p *DefaultStreamProvider) Advance(
	latest types.StreamPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	if latest > p.latest {
		p.latest = latest
	}
}

func (p *DefaultStreamProvider) LatestPosition(
	ctx context.Context,
) types.StreamPosition {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return p.latest
}
