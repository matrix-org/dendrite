package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type InviteStreamProvider struct {
	StreamProvider
}

func (p *InviteStreamProvider) StreamSetup() {
	p.StreamProvider.StreamSetup()

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	p.latest = 0
}

func (p *InviteStreamProvider) StreamLatestPosition(
	ctx context.Context,
) types.StreamingToken {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return types.StreamingToken{
		InvitePosition: p.latest,
	}
}

// nolint:gocyclo
func (p *InviteStreamProvider) StreamRange(
	ctx context.Context,
	req *types.StreamRangeRequest,
	from, to types.StreamingToken,
) (newPos types.StreamingToken) {

	return types.StreamingToken{
		InvitePosition: 0,
	}
}

func (p *InviteStreamProvider) StreamNotifyAfter(
	ctx context.Context,
	from types.StreamingToken,
) chan struct{} {
	ch := make(chan struct{})

	check := func() bool {
		p.latestMutex.RLock()
		defer p.latestMutex.RUnlock()
		if p.latest > from.InvitePosition {
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
	go func(p *InviteStreamProvider) {
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
