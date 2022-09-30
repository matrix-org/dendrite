package streams

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type InviteStreamProvider struct {
	DefaultStreamProvider
}

func (p *InviteStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseSnapshot,
) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	p.latest = p.latestPosition(ctx, snapshot)
}

func (p *InviteStreamProvider) latestPosition(
	ctx context.Context, snapshot storage.DatabaseSnapshot,
) types.StreamPosition {
	id, err := snapshot.MaxStreamPositionForAccountData(context.Background())
	if err != nil {
		panic(err)
	}
	return id
}

func (p *InviteStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseSnapshot,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.latestPosition(ctx, snapshot))
}

func (p *InviteStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseSnapshot,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	r := types.Range{
		From: from,
		To:   to,
	}

	invites, retiredInvites, err := snapshot.InviteEventsInRange(
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

	// When doing an initial sync, we don't want to add retired invites, as this
	// can add rooms we were invited to, but already left.
	if from == 0 {
		return to
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
