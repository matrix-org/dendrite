package streams

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type PartitionedStreamProvider struct {
	DB                 storage.Database
	latest             types.LogPosition
	latestMutex        sync.RWMutex
	subscriptions      map[string]*partitionedStreamSubscription // userid+deviceid
	subscriptionsMutex sync.Mutex
}

type partitionedStreamSubscription struct {
	ctx  context.Context
	from types.LogPosition
	ch   chan struct{}
}

func (p *PartitionedStreamProvider) Setup() {
	p.subscriptions = make(map[string]*partitionedStreamSubscription)
}

func (p *PartitionedStreamProvider) Advance(
	latest types.LogPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	if latest.IsAfter(&p.latest) {
		p.latest = latest

		p.subscriptionsMutex.Lock()
		for id, s := range p.subscriptions {
			select {
			case <-s.ctx.Done():
				close(s.ch)
				delete(p.subscriptions, id)
				continue
			default:
				if latest.IsAfter(&s.from) {
					close(s.ch)
					delete(p.subscriptions, id)
				}
			}
		}
		p.subscriptionsMutex.Unlock()
	}
}

func (p *PartitionedStreamProvider) LatestPosition(
	ctx context.Context,
) types.LogPosition {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return p.latest
}

func (p *PartitionedStreamProvider) NotifyAfter(
	ctx context.Context,
	device *userapi.Device,
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

	id := device.UserID + device.ID
	p.subscriptionsMutex.Lock()
	if s, ok := p.subscriptions[id]; ok {
		close(s.ch)
	}
	p.subscriptions[id] = &partitionedStreamSubscription{ctx, from, ch}
	p.subscriptionsMutex.Unlock()

	return ch
}
