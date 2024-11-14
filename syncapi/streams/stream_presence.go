// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package streams

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tidwall/gjson"

	"github.com/element-hq/dendrite/syncapi/notifier"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type PresenceStreamProvider struct {
	DefaultStreamProvider
	// cache contains previously sent presence updates to avoid unneeded updates
	cache    sync.Map
	notifier *notifier.Notifier
}

func (p *PresenceStreamProvider) Setup(
	ctx context.Context, snapshot storage.DatabaseTransaction,
) {
	p.DefaultStreamProvider.Setup(ctx, snapshot)

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := snapshot.MaxStreamPositionForPresence(ctx)
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *PresenceStreamProvider) CompleteSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
) types.StreamPosition {
	return p.IncrementalSync(ctx, snapshot, req, 0, p.LatestPosition(ctx))
}

func (p *PresenceStreamProvider) IncrementalSync(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) types.StreamPosition {
	// We pull out a larger number than the filter asks for, since we're filtering out events later
	presences, err := snapshot.PresenceAfter(ctx, from, synctypes.EventFilter{Limit: 1000})
	if err != nil {
		req.Log.WithError(err).Error("p.DB.PresenceAfter failed")
		return from
	}

	getPresenceForUsers, err := p.getNeededUsersFromRequest(ctx, req, presences)
	if err != nil {
		req.Log.WithError(err).Error("getNeededUsersFromRequest failed")
		return from
	}

	// Got no presence between range and no presence to get from the database
	if len(getPresenceForUsers) == 0 && len(presences) == 0 {
		return to
	}

	dbPresences, err := snapshot.GetPresences(ctx, getPresenceForUsers)
	if err != nil {
		req.Log.WithError(err).Error("unable to query presence for user")
		_ = snapshot.Rollback()
		return from
	}
	for _, presence := range dbPresences {
		presences[presence.UserID] = presence
	}

	lastPos := from
	for _, presence := range presences {
		if presence == nil {
			continue
		}
		// Ignore users we don't share a room with
		if req.Device.UserID != presence.UserID && !p.notifier.IsSharedUser(req.Device.UserID, presence.UserID) {
			continue
		}
		cacheKey := req.Device.UserID + req.Device.ID + presence.UserID
		pres, ok := p.cache.Load(cacheKey)
		if ok {
			// skip already sent presence
			prevPresence := pres.(*types.PresenceInternal)
			currentlyActive := prevPresence.CurrentlyActive()
			skip := prevPresence.Equals(presence) && currentlyActive && req.Device.UserID != presence.UserID
			_, membershipChange := req.MembershipChanges[presence.UserID]
			if skip && !membershipChange {
				req.Log.Tracef("Skipping presence, no change (%s)", presence.UserID)
				continue
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
			return from
		}

		req.Response.Presence.Events = append(req.Response.Presence.Events, synctypes.ClientEvent{
			Content: content,
			Sender:  presence.UserID,
			Type:    spec.MPresence,
		})
		if presence.StreamPos > lastPos {
			lastPos = presence.StreamPos
		}
		if len(req.Response.Presence.Events) == req.Filter.Presence.Limit {
			break
		}
		p.cache.Store(cacheKey, presence)
	}

	if len(req.Response.Presence.Events) == 0 {
		return to
	}

	return lastPos
}

func (p *PresenceStreamProvider) getNeededUsersFromRequest(ctx context.Context, req *types.SyncRequest, presences map[string]*types.PresenceInternal) ([]string, error) {
	getPresenceForUsers := []string{}
	// Add presence for users which newly joined a room
	for userID := range req.MembershipChanges {
		if _, ok := presences[userID]; ok {
			continue
		}
		getPresenceForUsers = append(getPresenceForUsers, userID)
	}

	// add newly joined rooms user presences
	newlyJoined := joinedRooms(req.Response, req.Device.UserID)
	if len(newlyJoined) == 0 {
		return getPresenceForUsers, nil
	}

	// TODO: Check if this is working better than before.
	if err := p.notifier.LoadRooms(ctx, p.DB, newlyJoined); err != nil {
		return getPresenceForUsers, fmt.Errorf("unable to refresh notifier lists: %w", err)
	}
	for _, roomID := range newlyJoined {
		roomUsers := p.notifier.JoinedUsers(roomID)
		for i := range roomUsers {
			// we already got a presence from this user
			if _, ok := presences[roomUsers[i]]; ok {
				continue
			}
			getPresenceForUsers = append(getPresenceForUsers, roomUsers[i])
		}
	}
	return getPresenceForUsers, nil
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

func membershipEventPresent(events []synctypes.ClientEvent, userID string) bool {
	for _, ev := range events {
		// it's enough to know that we have our member event here, don't need to check membership content
		// as it's implied by being in the respective section of the sync response.
		if ev.Type == spec.MRoomMember && ev.StateKey != nil && *ev.StateKey == userID {
			// ignore e.g. join -> join changes
			if gjson.GetBytes(ev.Unsigned, "prev_content.membership").Str == gjson.GetBytes(ev.Content, "membership").Str {
				continue
			}
			return true
		}
	}
	return false
}
