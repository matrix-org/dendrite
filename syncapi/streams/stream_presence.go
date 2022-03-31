// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package streams

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type PresenceStreamProvider struct {
	StreamProvider
	cache sync.Map
}

func (p *PresenceStreamProvider) Setup() {
	p.StreamProvider.Setup()

	id, err := p.DB.MaxStreamPositionForPresence(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *PresenceStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, req, 0, p.LatestPosition(ctx))
}

func (p *PresenceStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	presences, err := p.DB.PresenceAfter(ctx, from)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.PresenceAfter failed")
		return from
	}

	if len(presences) == 0 {
		return to
	}

	// get all joined users
	rooms, err := p.DB.AllJoinedUsersInRooms(ctx)
	if err != nil {
		req.Log.WithError(err).Error("unable to query joined users")
		return from
	}

	sharedUsers := map[string]bool{
		req.Device.UserID: true,
	}
	for roomID := range req.Rooms {
		roomUsers := rooms[roomID]
		for i := range roomUsers {
			sharedUsers[roomUsers[i]] = true
		}
	}

	// add newly joined rooms user presences
	newlyJoined := joinedRooms(req.Response, req.Device.UserID)
	for _, roomID := range newlyJoined {
		roomUsers := rooms[roomID]
		for i := range roomUsers {
			sharedUsers[roomUsers[i]] = true
			// we already got a presence from this user
			if _, ok := presences[roomUsers[i]]; ok {
				continue
			}
			presences[roomUsers[i]], err = p.DB.GetPresence(ctx, roomUsers[i])
			if err != nil {
				req.Log.WithError(err).Warn("unable to query presence for user")
				if err == sql.ErrNoRows {
					continue
				}
				return from
			}
		}
	}

	lastPos := to
	for i := range presences {
		presence := presences[i]
		// Ignore users we don't share a room with
		if !sharedUsers[presence.UserID] {
			continue
		}
		pres, ok := p.cache.Load(req.Device.UserID + presence.UserID)
		if ok {
			// skip already sent presence
			prevPresence := pres.(*types.Presence)
			currentlyActive := time.Since(prevPresence.LastActiveTS.Time()).Minutes() < 5
			samePresence := prevPresence.ClientFields.Presence == presence.ClientFields.Presence &&
				prevPresence.ClientFields.StatusMsg == presence.ClientFields.StatusMsg
			skip := currentlyActive && samePresence && req.Device.UserID != presence.UserID
			if skip {
				req.Log.Debugf("Skipping presence, no change (%s)", presence.UserID)
				continue
			}
		}
		presence.ClientFields.LastActiveAgo = time.Since(presence.LastActiveTS.Time()).Milliseconds()
		currentlyActive := time.Since(presence.LastActiveTS.Time()).Minutes() < 5
		if presence.ClientFields.Presence == "online" {
			presence.ClientFields.CurrentlyActive = &currentlyActive
		}

		content, err := json.Marshal(presence.ClientFields)
		if err != nil {
			return from
		}

		req.Response.Presence.Events = append(req.Response.Presence.Events, gomatrixserverlib.ClientEvent{
			Content: content,
			Sender:  presence.UserID,
			Type:    gomatrixserverlib.MPresence,
		})
		if presence.StreamPos > lastPos {
			lastPos = presence.StreamPos
		}
		p.cache.Store(req.Device.UserID+presence.UserID, presence)
	}

	return lastPos
}

func joinedRooms(res *types.Response, userID string) []string {
	var roomIDs []string
	for roomID, join := range res.Rooms.Join {
		// we would expect to see our join event somewhere if we newly joined the room.
		// Normal events get put in the join section so it's not enough to know the room ID is present in 'join'.
		newlyJoined := membershipEventPresent(join.State.Events, userID)
		if newlyJoined {
			roomIDs = append(roomIDs, roomID)
			continue
		}
		newlyJoined = membershipEventPresent(join.Timeline.Events, userID)
		if newlyJoined {
			roomIDs = append(roomIDs, roomID)
		}
	}
	return roomIDs
}

func membershipEventPresent(events []gomatrixserverlib.ClientEvent, userID string) bool {
	for _, ev := range events {
		// it's enough to know that we have our member event here, don't need to check membership content
		// as it's implied by being in the respective section of the sync response.
		if ev.Type == gomatrixserverlib.MRoomMember && ev.StateKey != nil && *ev.StateKey == userID {
			return true
		}
	}
	return false
}
