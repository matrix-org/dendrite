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

package consumers

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Shopify/sarama"
	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// OutputKeyChangeEventConsumer consumes events that originated in the key server.
type OutputKeyChangeEventConsumer struct {
	keyChangeConsumer   *internal.ContinualConsumer
	db                  storage.Database
	serverName          gomatrixserverlib.ServerName // our server name
	currentStateAPI     currentstateAPI.CurrentStateInternalAPI
	keyAPI              api.KeyInternalAPI
	partitionToOffset   map[int32]int64
	partitionToOffsetMu sync.Mutex
}

// NewOutputKeyChangeEventConsumer creates a new OutputKeyChangeEventConsumer.
// Call Start() to begin consuming from the key server.
func NewOutputKeyChangeEventConsumer(
	serverName gomatrixserverlib.ServerName,
	topic string,
	kafkaConsumer sarama.Consumer,
	keyAPI api.KeyInternalAPI,
	currentStateAPI currentstateAPI.CurrentStateInternalAPI,
	store storage.Database,
) *OutputKeyChangeEventConsumer {

	consumer := internal.ContinualConsumer{
		Topic:          topic,
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}

	s := &OutputKeyChangeEventConsumer{
		keyChangeConsumer:   &consumer,
		db:                  store,
		serverName:          serverName,
		keyAPI:              keyAPI,
		currentStateAPI:     currentStateAPI,
		partitionToOffset:   make(map[int32]int64),
		partitionToOffsetMu: sync.Mutex{},
	}

	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from the key server
func (s *OutputKeyChangeEventConsumer) Start() error {
	offsets, err := s.keyChangeConsumer.StartOffsets()
	s.partitionToOffsetMu.Lock()
	for _, o := range offsets {
		s.partitionToOffset[o.Partition] = o.Offset
	}
	s.partitionToOffsetMu.Unlock()
	return err
}

func (s *OutputKeyChangeEventConsumer) updateOffset(msg *sarama.ConsumerMessage) {
	s.partitionToOffsetMu.Lock()
	defer s.partitionToOffsetMu.Unlock()
	s.partitionToOffset[msg.Partition] = msg.Offset
}

func (s *OutputKeyChangeEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	defer func() {
		s.updateOffset(msg)
	}()
	var output api.DeviceKeys
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Error("syncapi: failed to unmarshal key change event from key server")
		return err
	}
	// work out who we need to notify about the new key
	var queryRes currentstateAPI.QuerySharedUsersResponse
	err := s.currentStateAPI.QuerySharedUsers(context.Background(), &currentstateAPI.QuerySharedUsersRequest{
		UserID: output.UserID,
	}, &queryRes)
	if err != nil {
		log.WithError(err).Error("syncapi: failed to QuerySharedUsers for key change event from key server")
		return err
	}
	// TODO: f.e queryRes.UserIDsToCount : notify users by waking up streams
	return nil
}

// Catchup fills in the given response for the given user ID to bring it up-to-date with device lists. hasNew=true if the response
// was filled in, else false if there are no new device list changes because there is nothing to catch up on. The response MUST
// be already filled in with join/leave information.
func (s *OutputKeyChangeEventConsumer) Catchup(
	ctx context.Context, userID string, res *types.Response, tok types.StreamingToken,
) (newTok *types.StreamingToken, hasNew bool, err error) {
	// Track users who we didn't track before but now do by virtue of sharing a room with them, or not.
	newlyJoinedRooms := joinedRooms(res, userID)
	newlyLeftRooms := leftRooms(res)
	if len(newlyJoinedRooms) > 0 || len(newlyLeftRooms) > 0 {
		changed, left, err := s.trackChangedUsers(ctx, userID, newlyJoinedRooms, newlyLeftRooms)
		if err != nil {
			return nil, false, err
		}
		res.DeviceLists.Changed = changed
		res.DeviceLists.Left = left
		hasNew = len(changed) > 0 || len(left) > 0
	}

	// now also track users who we already share rooms with but who have updated their devices between the two tokens
	// TODO: Extract partition/offset from sync token
	var partition int32
	var offset int64
	var queryRes api.QueryKeyChangesResponse
	s.keyAPI.QueryKeyChanges(ctx, &api.QueryKeyChangesRequest{
		Partition: partition,
		Offset:    offset,
	}, &queryRes)
	if queryRes.Error != nil {
		// don't fail the catchup because we may have got useful information by tracking membership
		util.GetLogger(ctx).WithError(queryRes.Error).Error("QueryKeyChanges failed")
	} else {
		// TODO: Make a new streaming token using the new offset
		userSet := make(map[string]bool)
		for _, userID := range res.DeviceLists.Changed {
			userSet[userID] = true
		}
		for _, userID := range queryRes.UserIDs {
			if !userSet[userID] {
				res.DeviceLists.Changed = append(res.DeviceLists.Changed, userID)
			}
		}
	}
	return
}

func (s *OutputKeyChangeEventConsumer) OnJoinEvent(ev *gomatrixserverlib.HeaderedEvent) {
	// work out who we are now sharing rooms with which we previously were not and notify them about the joining
	// users keys:
	changed, _, err := s.trackChangedUsers(context.Background(), *ev.StateKey(), []string{ev.RoomID()}, nil)
	if err != nil {
		log.WithError(err).Error("OnJoinEvent: failed to work out changed users")
		return
	}
	// TODO: f.e changed, wake up stream
	for _, userID := range changed {
		log.Infof("OnJoinEvent:Notify %s that %s should have device lists tracked", userID, *ev.StateKey())
	}
}

func (s *OutputKeyChangeEventConsumer) OnLeaveEvent(ev *gomatrixserverlib.HeaderedEvent) {
	// work out who we are no longer sharing any rooms with and notify them about the leaving user
	_, left, err := s.trackChangedUsers(context.Background(), *ev.StateKey(), nil, []string{ev.RoomID()})
	if err != nil {
		log.WithError(err).Error("OnLeaveEvent: failed to work out left users")
		return
	}
	// TODO: f.e left, wake up stream
	for _, userID := range left {
		log.Infof("OnLeaveEvent:Notify %s that %s should no longer track device lists", userID, *ev.StateKey())
	}

}

// nolint:gocyclo
func (s *OutputKeyChangeEventConsumer) trackChangedUsers(
	ctx context.Context, userID string, newlyJoinedRooms, newlyLeftRooms []string,
) (changed, left []string, err error) {
	// process leaves first, then joins afterwards so if we join/leave/join/leave we err on the side of including users.

	// Leave algorithm:
	// - Get set of users and number of times they appear in rooms prior to leave. - QuerySharedUsersRequest with 'IncludeRoomID'.
	// - Get users in newly left room. - QueryCurrentState
	// - Loop set of users and decrement by 1 for each user in newly left room.
	// - If count=0 then they share no more rooms so inform BOTH parties of this via 'left'=[...] in /sync.
	var queryRes currentstateAPI.QuerySharedUsersResponse
	err = s.currentStateAPI.QuerySharedUsers(ctx, &currentstateAPI.QuerySharedUsersRequest{
		UserID:         userID,
		IncludeRoomIDs: newlyLeftRooms,
	}, &queryRes)
	if err != nil {
		return nil, nil, err
	}
	var stateRes currentstateAPI.QueryBulkStateContentResponse
	err = s.currentStateAPI.QueryBulkStateContent(ctx, &currentstateAPI.QueryBulkStateContentRequest{
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
	err = s.currentStateAPI.QuerySharedUsers(ctx, &currentstateAPI.QuerySharedUsersRequest{
		UserID:         userID,
		ExcludeRoomIDs: newlyJoinedRooms,
	}, &queryRes)
	if err != nil {
		return nil, left, err
	}
	err = s.currentStateAPI.QueryBulkStateContent(ctx, &currentstateAPI.QueryBulkStateContentRequest{
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
