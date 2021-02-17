package streams

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type StreamProvider struct {
	DB          storage.Database
	latest      types.StreamPosition
	latestMutex sync.RWMutex
}

func (p *StreamProvider) Setup() {
}

func (p *StreamProvider) Advance(
	latest types.StreamPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	if latest > p.latest {
		p.latest = latest
	}
}

func (p *StreamProvider) LatestPosition(
	ctx context.Context,
) types.StreamPosition {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return p.latest
}
