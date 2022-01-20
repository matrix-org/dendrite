package streams

import (
	"context"
	"encoding/json"
	"time"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type PresenceStreamProvider struct {
	StreamProvider
	userAPI userapi.UserInternalAPI
	rsAPI   api.RoomserverInternalAPI
}

func (p *PresenceStreamProvider) Setup() {
	p.StreamProvider.Setup()

	res := userapi.QueryMaxPresenceIDResponse{}
	if err := p.userAPI.QueryMaxPresenceID(context.Background(), &userapi.QueryMaxPresenceIDRequest{}, &res); err != nil {
		panic(err)
	}
	p.latest = types.StreamPosition(res.ID)
}

func (p *PresenceStreamProvider) CompleteSync(ctx context.Context, req *types.SyncRequest) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

type outputPresence struct {
	AvatarUrl       string  `json:"avatar_url,omitempty"`
	CurrentlyActive bool    `json:"currently_active,omitempty"`
	LastActiveAgo   int64   `json:"last_active_ago,omitempty"`
	Presence        string  `json:"presence,omitempty"`
	StatusMsg       *string `json:"status_msg,omitempty"`
}

func (p *PresenceStreamProvider) IncrementalSync(ctx context.Context, req *types.SyncRequest, from, to types.StreamPosition) types.StreamPosition {
	res := userapi.QueryPresenceAfterResponse{}
	if err := p.userAPI.QueryPresenceAfter(ctx, &userapi.QueryPresenceAfterRequest{StreamPos: int64(from)}, &res); err != nil {
		req.Log.WithError(err).Error("unable to fetch presence after")
		return from
	}
	if len(res.Presences) == 0 {
		return to
	}
	evs := []gomatrixserverlib.ClientEvent{}
	var maxPos int64
	for _, presence := range res.Presences {
		ev := gomatrixserverlib.ClientEvent{}
		lastActive := time.Since(presence.LastActiveTS.Time())
		pres := outputPresence{
			CurrentlyActive: lastActive <= time.Minute*5,
			LastActiveAgo:   lastActive.Milliseconds(),
			Presence:        presence.Presence.String(),
			StatusMsg:       presence.StatusMsg,
		}

		j, err := json.Marshal(pres)
		if err != nil {
			req.Log.WithError(err).Error("json.Marshal failed")
			return from
		}
		ev.Type = gomatrixserverlib.MPresence
		ev.Sender = presence.UserID
		ev.Content = j

		evs = append(evs, ev)
		if presence.StreamPos > maxPos {
			maxPos = presence.StreamPos
		}
	}
	req.Response.Presence.Events = append(req.Response.Presence.Events, evs...)

	return types.StreamPosition(maxPos)
}
