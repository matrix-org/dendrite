package streams

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
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
		// skip ignored user events
		if _, ok := req.IgnoredUsers.List[inviteEvent.Sender()]; ok {
			continue
		}
		ir := types.NewInviteResponse(inviteEvent)
		req.Response.Rooms.Invite[roomID] = *ir
	}

	for roomID := range retiredInvites {
		if _, ok := req.Response.Rooms.Join[roomID]; !ok {
			lr := types.NewLeaveResponse()
			h := sha256.Sum256(append([]byte(roomID), []byte(strconv.FormatInt(int64(to), 10))...))
			lr.Timeline.Events = append(lr.Timeline.Events, gomatrixserverlib.ClientEvent{
				// fake event ID which muxes in the to position
				EventID:        "$" + base64.RawURLEncoding.EncodeToString(h[:]),
				OriginServerTS: gomatrixserverlib.AsTimestamp(time.Now()),
				RoomID:         roomID,
				Sender:         req.Device.UserID,
				StateKey:       &req.Device.UserID,
				Type:           "m.room.member",
				Content:        gomatrixserverlib.RawJSON(`{"membership":"leave"}`),
			})
			req.Response.Rooms.Leave[roomID] = *lr
		}
	}

	return to
}
