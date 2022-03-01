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

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	keytypes "github.com/matrix-org/dendrite/keyserver/types"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const DeviceListLogName = "dl"

// DeviceOTKCounts adds one-time key counts to the /sync response
func DeviceOTKCounts(ctx context.Context, keyAPI keyapi.KeyInternalAPI, userID, deviceID string, res *types.Response) error {
	var queryRes keyapi.QueryOneTimeKeysResponse
	keyAPI.QueryOneTimeKeys(ctx, &keyapi.QueryOneTimeKeysRequest{
		UserID:   userID,
		DeviceID: deviceID,
	}, &queryRes)
	if queryRes.Error != nil {
		return queryRes.Error
	}
	res.DeviceListsOTKCount = queryRes.Count.KeyCount
	return nil
}

// DeviceListCatchup fills in the given response for the given user ID to bring it up-to-date with device lists. hasNew=true if the response
// was filled in, else false if there are no new device list changes because there is nothing to catch up on. The response MUST
// be already filled in with join/leave information.
func DeviceListCatchup(
	ctx context.Context, keyAPI keyapi.KeyInternalAPI, rsAPI roomserverAPI.RoomserverInternalAPI,
	userID string, res *types.Response, from, to types.StreamPosition,
) (newPos types.StreamPosition, hasNew bool, err error) {

	// Track users who we didn't track before but now do by virtue of sharing a room with them, or not.
	newlyJoinedRooms := joinedRooms(res, userID)
	newlyLeftRooms := leftRooms(res)
	if len(newlyJoinedRooms) > 0 || len(newlyLeftRooms) > 0 {
		changed, left, err := TrackChangedUsers(ctx, rsAPI, userID, newlyJoinedRooms, newlyLeftRooms)
		if err != nil {
			return to, false, err
		}
		res.DeviceLists.Changed = changed
		res.DeviceLists.Left = left
		hasNew = len(changed) > 0 || len(left) > 0
	}

	// now also track users who we already share rooms with but who have updated their devices between the two tokens
	offset := keytypes.OffsetOldest
	toOffset := keytypes.OffsetNewest
	if to > 0 && to > from {
		toOffset = int64(to)
	}
	if from > 0 {
		offset = int64(from)
	}
	var queryRes keyapi.QueryKeyChangesResponse
	keyAPI.QueryKeyChanges(ctx, &keyapi.QueryKeyChangesRequest{
		Offset:   offset,
		ToOffset: toOffset,
	}, &queryRes)
	if queryRes.Error != nil {
		// don't fail the catchup because we may have got useful information by tracking membership
		util.GetLogger(ctx).WithError(queryRes.Error).Error("QueryKeyChanges failed")
		return to, hasNew, nil
	}

	// Work out which user IDs we care about — that includes those in the original request,
	// the response from QueryKeyChanges (which includes ALL users who have changed keys)
	// as well as every user who has a join or leave event in the current sync response. We
	// will request information about which rooms these users are joined to, so that we can
	// see if we still share any rooms with them.
	joinUserIDs, leaveUserIDs := membershipEvents(res)
	queryRes.UserIDs = append(queryRes.UserIDs, joinUserIDs...)
	queryRes.UserIDs = append(queryRes.UserIDs, leaveUserIDs...)
	queryRes.UserIDs = util.UniqueStrings(queryRes.UserIDs)
	var sharedUsersMap map[string]int
	sharedUsersMap, queryRes.UserIDs = filterSharedUsers(ctx, rsAPI, userID, queryRes.UserIDs)
	util.GetLogger(ctx).Debugf(
		"QueryKeyChanges request off=%d,to=%d response off=%d uids=%v",
		offset, toOffset, queryRes.Offset, queryRes.UserIDs,
	)
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
	// Finally, add in users who have joined or left.
	// TODO: This is sub-optimal because we will add users to `changed` even if we already shared a room with them.
	for _, userID := range joinUserIDs {
		if !userSet[userID] {
			res.DeviceLists.Changed = append(res.DeviceLists.Changed, userID)
			hasNew = true
			userSet[userID] = true
		}
	}
	for _, userID := range leaveUserIDs {
		if sharedUsersMap[userID] == 0 {
			// we no longer share a room with this user when they left, so add to left list.
			res.DeviceLists.Left = append(res.DeviceLists.Left, userID)
		}
	}

	return types.StreamPosition(queryRes.Offset), hasNew, nil
}

// TrackChangedUsers calculates the values of device_lists.changed|left in the /sync response.
func TrackChangedUsers(
	ctx context.Context, rsAPI roomserverAPI.RoomserverInternalAPI, userID string, newlyJoinedRooms, newlyLeftRooms []string,
) (changed, left []string, err error) {
	// process leaves first, then joins afterwards so if we join/leave/join/leave we err on the side of including users.

	// Leave algorithm:
	// - Get set of users and number of times they appear in rooms prior to leave. - QuerySharedUsersRequest with 'IncludeRoomID'.
	// - Get users in newly left room. - QueryCurrentState
	// - Loop set of users and decrement by 1 for each user in newly left room.
	// - If count=0 then they share no more rooms so inform BOTH parties of this via 'left'=[...] in /sync.
	var queryRes roomserverAPI.QuerySharedUsersResponse
	err = rsAPI.QuerySharedUsers(ctx, &roomserverAPI.QuerySharedUsersRequest{
		UserID:         userID,
		IncludeRoomIDs: newlyLeftRooms,
	}, &queryRes)
	if err != nil {
		return nil, nil, err
	}
	var stateRes roomserverAPI.QueryBulkStateContentResponse
	err = rsAPI.QueryBulkStateContent(ctx, &roomserverAPI.QueryBulkStateContentRequest{
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
	err = rsAPI.QuerySharedUsers(ctx, &roomserverAPI.QuerySharedUsersRequest{
		UserID:         userID,
		ExcludeRoomIDs: newlyJoinedRooms,
	}, &queryRes)
	if err != nil {
		return nil, left, err
	}
	err = rsAPI.QueryBulkStateContent(ctx, &roomserverAPI.QueryBulkStateContentRequest{
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

func filterSharedUsers(
	ctx context.Context, rsAPI roomserverAPI.RoomserverInternalAPI, userID string, usersWithChangedKeys []string,
) (map[string]int, []string) {
	var result []string
	var sharedUsersRes roomserverAPI.QuerySharedUsersResponse
	err := rsAPI.QuerySharedUsers(ctx, &roomserverAPI.QuerySharedUsersRequest{
		UserID:       userID,
		OtherUserIDs: usersWithChangedKeys,
	}, &sharedUsersRes)
	if err != nil {
		// default to all users so we do needless queries rather than miss some important device update
		return nil, usersWithChangedKeys
	}
	// We forcibly put ourselves in this list because we should be notified about our own device updates
	// and if we are in 0 rooms then we don't technically share any room with ourselves so we wouldn't
	// be notified about key changes.
	sharedUsersRes.UserIDsToCount[userID] = 1

	for _, uid := range usersWithChangedKeys {
		if sharedUsersRes.UserIDsToCount[uid] > 0 {
			result = append(result, uid)
		}
	}
	return sharedUsersRes.UserIDsToCount, result
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
func membershipEvents(res *types.Response) (joinUserIDs, leaveUserIDs []string) {
	for _, room := range res.Rooms.Join {
		for _, ev := range room.Timeline.Events {
			if ev.Type == gomatrixserverlib.MRoomMember && ev.StateKey != nil {
				if strings.Contains(string(ev.Content), `"join"`) {
					joinUserIDs = append(joinUserIDs, *ev.StateKey)
				} else if strings.Contains(string(ev.Content), `"invite"`) {
					joinUserIDs = append(joinUserIDs, *ev.StateKey)
				} else if strings.Contains(string(ev.Content), `"leave"`) {
					leaveUserIDs = append(leaveUserIDs, *ev.StateKey)
				} else if strings.Contains(string(ev.Content), `"ban"`) {
					leaveUserIDs = append(leaveUserIDs, *ev.StateKey)
				}
			}
		}
	}
	return
}
