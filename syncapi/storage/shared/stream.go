package shared

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type StreamProvider struct {
	DB          *Database
	latest      types.StreamPosition
	latestMutex sync.RWMutex
	update      *sync.Cond
}

func (p *StreamProvider) Setup() {
	locker := &sync.Mutex{}
	p.update = sync.NewCond(locker)
}

func (p *StreamProvider) Advance(
	latest types.StreamPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	if latest > p.latest {
		p.latest = latest
		p.update.Broadcast()
	}
}

func (p *StreamProvider) LatestPosition(
	ctx context.Context,
) types.StreamPosition {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return p.latest
}

func (p *StreamProvider) NotifyAfter(
	ctx context.Context,
	from types.StreamPosition,
) chan struct{} {
	ch := make(chan struct{})

	check := func() bool {
		p.latestMutex.RLock()
		defer p.latestMutex.RUnlock()
		if p.latest > from {
			close(ch)
			return true
		}
		return false
	}

	// If we've already advanced past the specified position
	// then return straight away.
	if check() {
		return ch
	}

	// If we haven't, then we'll subscribe to updates. The
	// sync.Cond will fire every time the latest position
	// updates, so we can check and see if we've advanced
	// past it.
	go func(p *StreamProvider) {
		p.update.L.Lock()
		defer p.update.L.Unlock()

		for {
			select {
			case <-ctx.Done():
				// The context has expired, so there's no point
				// in continuing to wait for the update.
				close(ch)
				return
			default:
				// The latest position has been advanced. Let's
				// see if it's advanced to the position we care
				// about. If it has then we'll return.
				p.update.Wait()
				if check() {
					return
				}
			}
		}
	}(p)

	return ch
}
