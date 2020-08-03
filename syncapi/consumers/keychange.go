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
	syncinternal "github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/storage"
	syncapi "github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
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
	notifier            *syncapi.Notifier
}

// NewOutputKeyChangeEventConsumer creates a new OutputKeyChangeEventConsumer.
// Call Start() to begin consuming from the key server.
func NewOutputKeyChangeEventConsumer(
	serverName gomatrixserverlib.ServerName,
	topic string,
	kafkaConsumer sarama.Consumer,
	n *syncapi.Notifier,
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
		notifier:            n,
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
	var output api.DeviceMessage
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
	posUpdate := types.NewStreamToken(0, 0, map[string]*types.LogPosition{
		syncinternal.DeviceListLogName: &types.LogPosition{
			Offset:    msg.Offset,
			Partition: msg.Partition,
		},
	})
	for userID := range queryRes.UserIDsToCount {
		s.notifier.OnNewKeyChange(posUpdate, userID, output.UserID)
	}
	return nil
}

func (s *OutputKeyChangeEventConsumer) OnJoinEvent(ev *gomatrixserverlib.HeaderedEvent) {
	// work out who we are now sharing rooms with which we previously were not and notify them about the joining
	// users keys:
	changed, _, err := syncinternal.TrackChangedUsers(context.Background(), s.currentStateAPI, *ev.StateKey(), []string{ev.RoomID()}, nil)
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
	_, left, err := syncinternal.TrackChangedUsers(context.Background(), s.currentStateAPI, *ev.StateKey(), nil, []string{ev.RoomID()})
	if err != nil {
		log.WithError(err).Error("OnLeaveEvent: failed to work out left users")
		return
	}
	// TODO: f.e left, wake up stream
	for _, userID := range left {
		log.Infof("OnLeaveEvent:Notify %s that %s should no longer track device lists", userID, *ev.StateKey())
	}

}
