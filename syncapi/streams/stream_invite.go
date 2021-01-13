package streams

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

	id, err := p.DB.MaxStreamPositionForInvites(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
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

	invites, retiredInvites, err := p.DB.InviteEventsInRange(
		ctx, req.Device.UserID, r,
	)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.InviteEventsInRange failed")
		return from
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
