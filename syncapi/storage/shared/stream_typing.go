package shared

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type TypingStreamProvider struct {
	DB          *Database
	latest      types.StreamPosition
	latestMutex sync.RWMutex
	update      *sync.Cond
}

func (p *TypingStreamProvider) StreamSetup() {
	locker := &sync.Mutex{}
	p.update = sync.NewCond(locker)
}

func (p *TypingStreamProvider) StreamAdvance(
	latest types.StreamPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	p.latest = latest
	p.update.Broadcast()
}

func (p *TypingStreamProvider) StreamRange(
	ctx context.Context,
	res *types.Response,
	device *userapi.Device,
	from, to types.StreamingToken,
	filter gomatrixserverlib.EventFilter,
) types.StreamingToken {
	return types.StreamingToken{}
}

func (p *TypingStreamProvider) StreamNotifyAfter(
	ctx context.Context,
	from types.StreamingToken,
) chan struct{} {
	ch := make(chan struct{})

	check := func() bool {
		p.latestMutex.RLock()
		defer p.latestMutex.RUnlock()
		if p.latest > from.TypingPosition {
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
	go func(p *TypingStreamProvider) {
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

func (p *TypingStreamProvider) StreamLatestPosition(
	ctx context.Context,
) types.StreamingToken {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return types.StreamingToken{
		TypingPosition: p.latest,
	}
}
