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
	"fmt"
	"sync"

	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type PresenceStreamProvider struct {
	StreamProvider
	// cache contains previously sent presence updates to avoid unneeded updates
	cache    sync.Map
	notifier *notifier.Notifier
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
	presences, latest, err := p.DB.RecentPresence(ctx)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.RecentPresence failed")
		return 0
	}
	if len(presences) == 0 {
		return latest
	}
	if err := p.populatePresence(ctx, req, presences, true); err != nil {
		return 0
	}
	return latest
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
	if err := p.populatePresence(ctx, req, presences, false); err != nil {
		return from
	}
	return to
}

func (p *PresenceStreamProvider) populatePresence(
	ctx context.Context,
	req *types.SyncRequest,
	presences map[string]*types.PresenceInternal,
	ignoreCache bool,
) error {
	// add newly joined rooms user presences
	newlyJoined := joinedRooms(req.Response, req.Device.UserID)
	if len(newlyJoined) > 0 {
		// TODO: This refreshes all lists and is quite expensive
		// The notifier should update the lists itself
		if err := p.notifier.Load(ctx, p.DB); err != nil {
			return err
		}
		for _, roomID := range newlyJoined {
			roomUsers := p.notifier.JoinedUsers(roomID)
			for i := range roomUsers {
				// we already got a presence from this user
				if _, ok := presences[roomUsers[i]]; ok {
					continue
				}
				var err error
				presences[roomUsers[i]], err = p.DB.GetPresence(ctx, roomUsers[i])
				if err != nil {
					if err == sql.ErrNoRows {
						continue
					}
					return err
				}
			}
		}
	}

	for i := range presences {
		presence := presences[i]
		// Ignore users we don't share a room with
		if req.Device.UserID != presence.UserID && !p.notifier.IsSharedUser(req.Device.UserID, presence.UserID) {
			continue
		}
		cacheKey := req.Device.UserID + req.Device.ID + presence.UserID
		if !ignoreCache {
			pres, ok := p.cache.Load(cacheKey)
			if ok {
				// skip already sent presence
				prevPresence := pres.(*types.PresenceInternal)
				currentlyActive := prevPresence.CurrentlyActive()
				if prevPresence.Equals(presence) && currentlyActive && req.Device.UserID != presence.UserID {
					continue
				}
			}
		}

		if _, known := types.PresenceFromString(presence.ClientFields.Presence); known {
			presence.ClientFields.LastActiveAgo = presence.LastActiveAgo()
			if presence.ClientFields.Presence == "online" {
				currentlyActive := presence.CurrentlyActive()
				presence.ClientFields.CurrentlyActive = &currentlyActive
			}
		} else {
			presence.ClientFields.Presence = "offline"
		}

		content, err := json.Marshal(presence.ClientFields)
		if err != nil {
			return fmt.Errorf("json.Unmarshal: %w", err)
		}

		req.Response.Presence.Events = append(req.Response.Presence.Events, gomatrixserverlib.ClientEvent{
			Content: content,
			Sender:  presence.UserID,
			Type:    gomatrixserverlib.MPresence,
		})
		p.cache.Store(cacheKey, presence)
	}

	return nil
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
