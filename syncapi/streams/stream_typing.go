package streams

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type TypingStreamProvider struct {
	StreamProvider
	EDUCache *cache.EDUCache
}

func (p *TypingStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

func (p *TypingStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	var err error
	for roomID, membership := range req.Rooms {
		if membership != gomatrixserverlib.Join {
			continue
		}

		jr := *types.NewJoinResponse()
		if existing, ok := req.Response.Rooms.Join[roomID]; ok {
			jr = existing
		}

		if users, updated := p.EDUCache.GetTypingUsersIfUpdatedAfter(
			roomID, int64(from),
		); updated {
			ev := gomatrixserverlib.ClientEvent{
				Type: gomatrixserverlib.MTyping,
			}
			ev.Content, err = json.Marshal(map[string]interface{}{
				"user_ids": users,
			})
			if err != nil {
				req.Log.WithError(err).Error("json.Marshal failed")
				return from
			}

			jr.Ephemeral.Events = append(jr.Ephemeral.Events, ev)
			req.Response.Rooms.Join[roomID] = jr
		}
	}

	return to
}
