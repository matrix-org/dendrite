package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type PDUStreamProvider struct {
	StreamProvider
}

func (p *PDUStreamProvider) Setup() {
	p.StreamProvider.Setup()

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := p.DB.OutputEvents.SelectMaxEventID(context.Background(), nil)
	if err != nil {
		return
	}
	p.latest = types.StreamPosition(id)
}

// nolint:gocyclo
func (p *PDUStreamProvider) Range(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) (newPos types.StreamPosition) {
	r := types.Range{
		From:      from,
		To:        to,
		Backwards: from > to,
	}
	newPos = to

	var err error
	var events []types.StreamEvent
	var stateDeltas []stateDelta
	var joinedRooms []string

	// TODO: use filter provided in request
	stateFilter := gomatrixserverlib.DefaultStateFilter()

	if req.WantFullState {
		if stateDeltas, joinedRooms, err = p.DB.getStateDeltasForFullStateSync(ctx, req.Device, nil, r, req.Device.UserID, &stateFilter); err != nil {
			return
		}
	} else {
		if stateDeltas, joinedRooms, err = p.DB.getStateDeltas(ctx, req.Device, nil, r, req.Device.UserID, &stateFilter); err != nil {
			return
		}
	}

	for _, roomID := range joinedRooms {
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	for _, stateDelta := range stateDeltas {
		roomID := stateDelta.roomID
		room := types.JoinResponse{}

		if r.Backwards {
			// When using backward ordering, we want the most recent events first.
			if events, _, err = p.DB.OutputEvents.SelectRecentEvents(ctx, nil, roomID, r, req.Filter.Limit, false, false); err != nil {
				return
			}
		} else {
			// When using forward ordering, we want the least recent events first.
			if events, err = p.DB.OutputEvents.SelectEarlyEvents(ctx, nil, roomID, r, req.Filter.Limit); err != nil {
				return
			}
		}

		for _, event := range p.DB.StreamEventsToEvents(req.Device, events) {
			room.Timeline.Events = append(
				room.Timeline.Events,
				gomatrixserverlib.ToClientEvent(
					event.Event,
					gomatrixserverlib.FormatSync,
				),
			)
		}

		for _, event := range events {
			if event.StreamPosition > newPos {
				newPos = event.StreamPosition
			}
		}

		room.State.Events = gomatrixserverlib.HeaderedToClientEvents(
			stateDelta.stateEvents,
			gomatrixserverlib.FormatSync,
		)

		if len(events) > 0 {
			prevBatch, err := p.DB.getBackwardTopologyPos(ctx, nil, events)
			if err != nil {
				return
			}
			room.Timeline.PrevBatch = &prevBatch
		}

		req.Response.Rooms.Join[roomID] = room
	}

	return newPos
}
