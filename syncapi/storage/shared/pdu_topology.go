package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type PDUTopologyProvider struct {
	DB *Database
}

func (p *PDUTopologyProvider) TopologyRange(ctx context.Context, res *types.Response, roomID string, from, to types.TopologyToken, filter gomatrixserverlib.EventFilter) {
	backwardOrdering := from.Depth > to.Depth || from.PDUPosition > to.PDUPosition

	events, err := p.DB.GetEventsInTopologicalRange(ctx, &from, &to, roomID, filter.Limit, backwardOrdering)
	if err != nil {
		return
	}

	_ = events
}

func (p *PDUTopologyProvider) TopologyLatestPosition(ctx context.Context, roomID string) types.StreamPosition {
	token, err := p.DB.MaxTopologicalPosition(ctx, roomID)
	if err != nil {
		return 0
	}
	return token.PDUPosition
}
