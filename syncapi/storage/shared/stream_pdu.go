package shared

import (
	"context"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type PDUStreamProvider struct {
	DB          *Database
	latest      types.StreamPosition
	latestMutex sync.RWMutex
	update      *sync.Cond
}

func (p *PDUStreamProvider) StreamSetup() {
	locker := &sync.Mutex{}
	p.update = sync.NewCond(locker)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := p.DB.OutputEvents.SelectMaxEventID(context.Background(), nil)
	if err != nil {
		return
	}
	p.latest = types.StreamPosition(id)
}

func (p *PDUStreamProvider) StreamAdvance(
	latest types.StreamPosition,
) {
	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	p.latest = latest
	p.update.Broadcast()
}

func (p *PDUStreamProvider) StreamRange(
	ctx context.Context,
	res *types.Response,
	device *userapi.Device,
	from, to types.StreamingToken,
	filter gomatrixserverlib.EventFilter,
) (newPos types.StreamingToken) {
	r := types.Range{
		From:      from.PDUPosition,
		To:        to.PDUPosition,
		Backwards: from.IsAfter(to),
	}
	newPos = types.StreamingToken{
		PDUPosition: from.PDUPosition,
	}

	var err error
	var events []types.StreamEvent
	var stateDeltas []stateDelta

	// TODO: use filter provided in request
	stateFilter := gomatrixserverlib.DefaultStateFilter()

	if from.IsEmpty() {
		if stateDeltas, _, err = p.DB.getStateDeltas(ctx, device, nil, r, device.UserID, &stateFilter); err != nil {
			return
		}
	} else {
		if stateDeltas, _, err = p.DB.getStateDeltasForFullStateSync(ctx, device, nil, r, device.UserID, &stateFilter); err != nil {
			return
		}
	}

	for _, stateDelta := range stateDeltas {
		roomID := stateDelta.roomID
		room := types.JoinResponse{}

		if r.Backwards {
			// When using backward ordering, we want the most recent events first.
			if events, _, err = p.DB.OutputEvents.SelectRecentEvents(ctx, nil, roomID, r, filter.Limit, false, false); err != nil {
				return
			}
		} else {
			// When using forward ordering, we want the least recent events first.
			if events, err = p.DB.OutputEvents.SelectEarlyEvents(ctx, nil, roomID, r, filter.Limit); err != nil {
				return
			}
		}

		for _, event := range events {
			room.Timeline.Events = append(
				room.Timeline.Events,
				gomatrixserverlib.ToClientEvent(
					event.Event,
					gomatrixserverlib.FormatSync,
				),
			)

			if event.StreamPosition > newPos.PDUPosition {
				newPos.PDUPosition = event.StreamPosition
			}
		}

		room.State.Events = gomatrixserverlib.HeaderedToClientEvents(
			stateDelta.stateEvents,
			gomatrixserverlib.FormatSync,
		)

		// TODO: fill in prev_batch

		res.Rooms.Join[roomID] = room
	}

	return newPos
}

func (p *PDUStreamProvider) StreamNotifyAfter(
	ctx context.Context,
	from types.StreamingToken,
) chan struct{} {
	ch := make(chan struct{})

	check := func() bool {
		p.latestMutex.RLock()
		defer p.latestMutex.RUnlock()
		if p.latest > from.PDUPosition {
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
	go func(p *PDUStreamProvider) {
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

func (p *PDUStreamProvider) StreamLatestPosition(
	ctx context.Context,
) types.StreamingToken {
	p.latestMutex.RLock()
	defer p.latestMutex.RUnlock()

	return types.StreamingToken{
		PDUPosition: p.latest,
	}
}
