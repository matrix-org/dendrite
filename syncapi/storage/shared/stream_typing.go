package shared

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/types"
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

	if latest > p.latest {
		p.latest = latest
		p.update.Broadcast()
	}
}

func (p *TypingStreamProvider) StreamRange(
	ctx context.Context,
	req *types.StreamRangeRequest,
	from, to types.StreamingToken,
) types.StreamingToken {
	var err error
	for roomID := range req.Rooms {
		// This may have already been set by a previous stream, so
		// reuse it if it exists.
		jr := req.Response.Rooms.Join[roomID]

		if users, updated := p.DB.EDUCache.GetTypingUsersIfUpdatedAfter(
			roomID, int64(from.TypingPosition),
		); updated {
			ev := gomatrixserverlib.ClientEvent{
				Type: gomatrixserverlib.MTyping,
			}
			ev.Content, err = json.Marshal(map[string]interface{}{
				"user_ids": users,
			})
			if err != nil {
				return types.StreamingToken{
					TypingPosition: from.TypingPosition,
				}
			}

			jr.Ephemeral.Events = append(jr.Ephemeral.Events, ev)
			req.Response.Rooms.Join[roomID] = jr
		}
	}

	return types.StreamingToken{
		TypingPosition: types.StreamPosition(p.DB.EDUCache.GetLatestSyncPosition()),
	}
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
