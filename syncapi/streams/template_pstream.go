package streams

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type PartitionedStreamProvider struct {
	DB          storage.Database
	latest      types.LogPosition
	latestMutex sync.RWMutex
}

func (p *PartitionedStreamProvider) Setup() {
}

func (p *PartitionedStreamProvider) Advance(
	latest types.LogPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	if latest.IsAfter(&p.latest) {
		p.latest = latest
	}
}

func (p *PartitionedStreamProvider) LatestPosition(
	ctx context.Context,
) types.LogPosition {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return p.latest
}
