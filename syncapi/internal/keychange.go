// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package internal

import (
	"context"
	"strings"

	"github.com/Shopify/sarama"
	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/keyserver/api"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const DeviceListLogName = "dl"

// DeviceListCatchup fills in the given response for the given user ID to bring it up-to-date with device lists. hasNew=true if the response
// was filled in, else false if there are no new device list changes because there is nothing to catch up on. The response MUST
// be already filled in with join/leave information.
func DeviceListCatchup(
	ctx context.Context, keyAPI keyapi.KeyInternalAPI, stateAPI currentstateAPI.CurrentStateInternalAPI,
	userID string, res *types.Response, from, to types.StreamingToken,
) (hasNew bool, err error) {
	// Track users who we didn't track before but now do by virtue of sharing a room with them, or not.
	newlyJoinedRooms := joinedRooms(res, userID)
	newlyLeftRooms := leftRooms(res)
	if len(newlyJoinedRooms) > 0 || len(newlyLeftRooms) > 0 {
		changed, left, err := TrackChangedUsers(ctx, stateAPI, userID, newlyJoinedRooms, newlyLeftRooms)
		if err != nil {
			return false, err
		}
		res.DeviceLists.Changed = changed
		res.DeviceLists.Left = left
		hasNew = len(changed) > 0 || len(left) > 0
	}

	// now also track users who we already share rooms with but who have updated their devices between the two tokens

	var partition int32
	var offset int64
	// Extract partition/offset from sync token
	// TODO: In a world where keyserver is sharded there will be multiple partitions and hence multiple QueryKeyChanges to make.
	logOffset := from.Log(DeviceListLogName)
	if logOffset != nil {
		partition = logOffset.Partition
		offset = logOffset.Offset
	} else {
		partition = -1
		offset = sarama.OffsetOldest
	}
	var toOffset int64
	toLog := to.Log(DeviceListLogName)
	if toLog != nil {
		toOffset = toLog.Offset
	} else {
		toOffset = sarama.OffsetNewest
	}
	var queryRes api.QueryKeyChangesResponse
	keyAPI.QueryKeyChanges(ctx, &api.QueryKeyChangesRequest{
		Partition: partition,
		Offset:    offset,
		ToOffset:  toOffset,
	}, &queryRes)
	if queryRes.Error != nil {
		// don't fail the catchup because we may have got useful information by tracking membership
		util.GetLogger(ctx).WithError(queryRes.Error).Error("QueryKeyChanges failed")
		return hasNew, nil
	}
	userSet := make(map[string]bool)
	for _, userID := range res.DeviceLists.Changed {
		userSet[userID] = true
	}
	for _, userID := range queryRes.UserIDs {
		if !userSet[userID] {
			res.DeviceLists.Changed = append(res.DeviceLists.Changed, userID)
			hasNew = true
			userSet[userID] = true
		}
	}
	// if the response has any join/leave events, add them now.
	// TODO: This is sub-optimal because we will add users to `changed` even if we already shared a room with them.
	for _, userID := range membershipEvents(res) {
		if !userSet[userID] {
			res.DeviceLists.Changed = append(res.DeviceLists.Changed, userID)
			hasNew = true
			userSet[userID] = true
		}
	}
	return hasNew, nil
}

// TrackChangedUsers calculates the values of device_lists.changed|left in the /sync response.
// nolint:gocyclo
func TrackChangedUsers(
	ctx context.Context, stateAPI currentstateAPI.CurrentStateInternalAPI, userID string, newlyJoinedRooms, newlyLeftRooms []string,
) (changed, left []string, err error) {
	// process leaves first, then joins afterwards so if we join/leave/join/leave we err on the side of including users.

	// Leave algorithm:
	// - Get set of users and number of times they appear in rooms prior to leave. - QuerySharedUsersRequest with 'IncludeRoomID'.
	// - Get users in newly left room. - QueryCurrentState
	// - Loop set of users and decrement by 1 for each user in newly left room.
	// - If count=0 then they share no more rooms so inform BOTH parties of this via 'left'=[...] in /sync.
	var queryRes currentstateAPI.QuerySharedUsersResponse
	err = stateAPI.QuerySharedUsers(ctx, &currentstateAPI.QuerySharedUsersRequest{
		UserID:         userID,
		IncludeRoomIDs: newlyLeftRooms,
	}, &queryRes)
	if err != nil {
		return nil, nil, err
	}
	var stateRes currentstateAPI.QueryBulkStateContentResponse
	err = stateAPI.QueryBulkStateContent(ctx, &currentstateAPI.QueryBulkStateContentRequest{
		RoomIDs: newlyLeftRooms,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			{
				EventType: gomatrixserverlib.MRoomMember,
				StateKey:  "*",
			},
		},
		AllowWildcards: true,
	}, &stateRes)
	if err != nil {
		return nil, nil, err
	}
	for _, state := range stateRes.Rooms {
		for tuple, membership := range state {
			if membership != gomatrixserverlib.Join {
				continue
			}
			queryRes.UserIDsToCount[tuple.StateKey]--
		}
	}
	for userID, count := range queryRes.UserIDsToCount {
		if count <= 0 {
			left = append(left, userID) // left is returned
		}
	}

	// Join algorithm:
	// - Get the set of all joined users prior to joining room - QuerySharedUsersRequest with 'ExcludeRoomID'.
	// - Get users in newly joined room - QueryCurrentState
	// - Loop set of users in newly joined room, do they appear in the set of users prior to joining?
	// - If yes: then they already shared a room in common, do nothing.
	// - If no: then they are a brand new user so inform BOTH parties of this via 'changed=[...]'
	err = stateAPI.QuerySharedUsers(ctx, &currentstateAPI.QuerySharedUsersRequest{
		UserID:         userID,
		ExcludeRoomIDs: newlyJoinedRooms,
	}, &queryRes)
	if err != nil {
		return nil, left, err
	}
	err = stateAPI.QueryBulkStateContent(ctx, &currentstateAPI.QueryBulkStateContentRequest{
		RoomIDs: newlyJoinedRooms,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			{
				EventType: gomatrixserverlib.MRoomMember,
				StateKey:  "*",
			},
		},
		AllowWildcards: true,
	}, &stateRes)
	if err != nil {
		return nil, left, err
	}
	for _, state := range stateRes.Rooms {
		for tuple, membership := range state {
			if membership != gomatrixserverlib.Join {
				continue
			}
			// new user who we weren't previously sharing rooms with
			if _, ok := queryRes.UserIDsToCount[tuple.StateKey]; !ok {
				changed = append(changed, tuple.StateKey) // changed is returned
			}
		}
	}
	return changed, left, nil
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

func leftRooms(res *types.Response) []string {
	roomIDs := make([]string, len(res.Rooms.Leave))
	i := 0
	for roomID := range res.Rooms.Leave {
		roomIDs[i] = roomID
		i++
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

// returns the user IDs of anyone joining or leaving a room in this response. These users will be added to
// the 'changed' property because of https://matrix.org/docs/spec/client_server/r0.6.1#id84
// "For optimal performance, Alice should be added to changed in Bob's sync only when she adds a new device,
// or when Alice and Bob now share a room but didn't share any room previously. However, for the sake of simpler
// logic, a server may add Alice to changed when Alice and Bob share a new room, even if they previously already shared a room."
func membershipEvents(res *types.Response) (userIDs []string) {
	for _, room := range res.Rooms.Join {
		for _, ev := range room.Timeline.Events {
			if ev.Type == gomatrixserverlib.MRoomMember && ev.StateKey != nil {
				if strings.Contains(string(ev.Content), `"join"`) {
					userIDs = append(userIDs, *ev.StateKey)
				} else if strings.Contains(string(ev.Content), `"leave"`) {
					userIDs = append(userIDs, *ev.StateKey)
				} else if strings.Contains(string(ev.Content), `"ban"`) {
					userIDs = append(userIDs, *ev.StateKey)
				}
			}
		}
	}
	return
}
