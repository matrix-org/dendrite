package streams

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type TypingStreamProvider struct {
	DefaultStreamProvider
	EDUCache *caching.EDUCache
}

func (p *TypingStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.LatestPosition(ctx))
}

func (p *TypingStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	var err error
	for roomID, membership := range req.Rooms {
		if membership != gomatrixserverlib.Join {
			continue
		}

		jr, ok := req.Response.Rooms.Join[roomID]
		if !ok {
			jr = types.NewJoinResponse()
		}

		if users, updated := p.EDUCache.GetTypingUsersIfUpdatedAfter(
			roomID, int64(from),
		); updated {
			typingUsers := make([]string, 0, len(users))
			for i := range users {
				// skip ignored user events
				if _, ok := req.IgnoredUsers.List[users[i]]; !ok {
					typingUsers = append(typingUsers, users[i])
				}
			}
			ev := gomatrixserverlib.ClientEvent{
				Type: gomatrixserverlib.MTyping,
			}
			ev.Content, err = json.Marshal(map[string]interface{}{
				"user_ids": typingUsers,
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
