package shared

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
)

type InviteStreamProvider struct {
	StreamProvider
}

func (p *InviteStreamProvider) Setup() {
	p.StreamProvider.Setup()

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	latest, err := p.DB.Invites.SelectMaxInviteID(context.Background(), nil)
	if err != nil {
		return
	}

	p.latest = types.StreamPosition(latest)
}

func (p *InviteStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

func (p *InviteStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	r := types.Range{
		From: from,
		To:   to,
	}

	invites, retiredInvites, err := p.DB.Invites.SelectInviteEventsInRange(
		ctx, nil, req.Device.UserID, r,
	)
	if err != nil {
		return to // fmt.Errorf("d.Invites.SelectInviteEventsInRange: %w", err)
	}

	for roomID, inviteEvent := range invites {
		ir := types.NewInviteResponse(inviteEvent)
		req.Response.Rooms.Invite[roomID] = *ir
	}

	for roomID := range retiredInvites {
		if _, ok := req.Response.Rooms.Join[roomID]; !ok {
			lr := types.NewLeaveResponse()
			req.Response.Rooms.Leave[roomID] = *lr
		}
	}

	return to
}
