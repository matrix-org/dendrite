package streams

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type StreamProvider struct {
	DB                 storage.Database
	latest             types.StreamPosition
	latestMutex        sync.RWMutex
	subscriptions      map[string]*streamSubscription // userid+deviceid
	subscriptionsMutex sync.Mutex
}

type streamSubscription struct {
	ctx  context.Context
	from types.StreamPosition
	ch   chan struct{}
}

func (p *StreamProvider) Setup() {
	p.subscriptions = make(map[string]*streamSubscription)
}

func (p *StreamProvider) Advance(
	latest types.StreamPosition,
) {
	p.latestMutex.Lock()
	if latest > p.latest {
		p.latest = latest
	}
	p.latestMutex.Unlock()

	p.subscriptionsMutex.Lock()
	defer p.subscriptionsMutex.Unlock()

	for id, s := range p.subscriptions {
		select {
		case <-s.ctx.Done():
			close(s.ch)
			delete(p.subscriptions, id)
		default:
			if latest > s.from {
				close(s.ch)
				delete(p.subscriptions, id)
			}
		}
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
	device *userapi.Device,
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

	id := device.UserID + device.ID
	p.subscriptionsMutex.Lock()
	if s, ok := p.subscriptions[id]; ok {
		close(s.ch)
	}
	p.subscriptions[id] = &streamSubscription{ctx, from, ch}
	p.subscriptionsMutex.Unlock()

	return ch
}
