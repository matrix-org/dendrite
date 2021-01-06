package shared

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type StreamLogProvider struct {
	DB          *Database
	latest      types.LogPosition
	latestMutex sync.RWMutex
	update      *sync.Cond
}

func (p *StreamLogProvider) Setup() {
	locker := &sync.Mutex{}
	p.update = sync.NewCond(locker)
}

func (p *StreamLogProvider) Advance(
	latest types.LogPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	if latest.IsAfter(&p.latest) {
		p.latest = latest
		p.update.Broadcast()
	}
}

func (p *StreamLogProvider) LatestPosition(
	ctx context.Context,
) types.LogPosition {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return p.latest
}

func (p *StreamLogProvider) NotifyAfter(
	ctx context.Context,
	from types.LogPosition,
) chan struct{} {
	ch := make(chan struct{})

	check := func() bool {
		p.latestMutex.RLock()
		defer p.latestMutex.RUnlock()
		if p.latest.IsAfter(&from) {
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
	go func(p *StreamLogProvider) {
		p.update.L.Lock()
		defer p.update.L.Unlock()

		for {
			select {
			case <-ctx.Done():
				// The context has expired, so there's no point
				// in continuing to wait for the update.
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
