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
	"github.com/sirupsen/logrus"
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
	latest := p.LatestPosition(ctx)
	presences, _, err := p.DB.RecentPresence(ctx)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.RecentPresence failed")
		return latest
	}
	if err = p.populatePresence(ctx, req, presences, true); err != nil {
		logrus.WithError(err).Errorf("Failed to populate presence")
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
	if err = p.populatePresence(ctx, req, presences, false); err != nil {
		logrus.WithError(err).Errorf("Failed to populate presence")
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
	for _, room := range req.Response.Rooms.Join {
		for _, stateEvent := range append(room.State.Events, room.Timeline.Events...) {
			switch {
			case stateEvent.Type != gomatrixserverlib.MRoomMember:
				continue
			case stateEvent.StateKey == nil:
				continue
			}
			var memberContent gomatrixserverlib.MemberContent
			err := json.Unmarshal(stateEvent.Content, &memberContent)
			if err != nil {
				continue
			}
			if memberContent.Membership != gomatrixserverlib.Join {
				continue
			}
			userID := *stateEvent.StateKey
			presences[userID], err = p.DB.GetPresence(ctx, userID)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
	}

	for _, presence := range presences {
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
