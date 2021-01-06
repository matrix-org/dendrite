package shared

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type TypingStreamProvider struct {
	StreamProvider
}

func (p *TypingStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	// It isn't beneficial to send previous typing notifications
	// after a complete sync, so just return the latest position
	// and otherwise do nothing.
	return p.LatestPosition(ctx)
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

		jr := req.Response.Rooms.Join[roomID]

		if users, updated := p.DB.EDUCache.GetTypingUsersIfUpdatedAfter(
			roomID, int64(from),
		); updated {
			ev := gomatrixserverlib.ClientEvent{
				Type: gomatrixserverlib.MTyping,
			}
			ev.Content, err = json.Marshal(map[string]interface{}{
				"user_ids": users,
			})
			if err != nil {
				return to
			}

			jr.Ephemeral.Events = append(jr.Ephemeral.Events, ev)
			req.Response.Rooms.Join[roomID] = jr
		}
	}

	return to
}
