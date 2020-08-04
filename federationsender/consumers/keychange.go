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
	"fmt"

	"github.com/Shopify/sarama"
	stateapi "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// KeyChangeConsumer consumes events that originate in key server.
type KeyChangeConsumer struct {
	consumer   *internal.ContinualConsumer
	db         storage.Database
	queues     *queue.OutgoingQueues
	serverName gomatrixserverlib.ServerName
	stateAPI   stateapi.CurrentStateInternalAPI
}

// NewKeyChangeConsumer creates a new KeyChangeConsumer. Call Start() to begin consuming from key servers.
func NewKeyChangeConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	queues *queue.OutgoingQueues,
	store storage.Database,
	stateAPI stateapi.CurrentStateInternalAPI,
) *KeyChangeConsumer {
	c := &KeyChangeConsumer{
		consumer: &internal.ContinualConsumer{
			Topic:          string(cfg.Kafka.Topics.OutputKeyChangeEvent),
			Consumer:       kafkaConsumer,
			PartitionStore: store,
		},
		queues:     queues,
		db:         store,
		serverName: cfg.Matrix.ServerName,
		stateAPI:   stateAPI,
	}
	c.consumer.ProcessMessage = c.onMessage

	return c
}

// Start consuming from key servers
func (t *KeyChangeConsumer) Start() error {
	if err := t.consumer.Start(); err != nil {
		return fmt.Errorf("t.consumer.Start: %w", err)
	}
	return nil
}

// onMessage is called in response to a message received on the
// key change events topic from the key server.
func (t *KeyChangeConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var m api.DeviceMessage
	if err := json.Unmarshal(msg.Value, &m); err != nil {
		log.WithError(err).Errorf("failed to read device message from key change topic")
		return nil
	}
	logger := log.WithField("user_id", m.UserID)

	// only send key change events which originated from us
	_, originServerName, err := gomatrixserverlib.SplitID('@', m.UserID)
	if err != nil {
		logger.WithError(err).Error("Failed to extract domain from key change event")
		return nil
	}
	if originServerName != t.serverName {
		return nil
	}

	var queryRes stateapi.QueryRoomsForUserResponse
	err = t.stateAPI.QueryRoomsForUser(context.Background(), &stateapi.QueryRoomsForUserRequest{
		UserID:         m.UserID,
		WantMembership: "join",
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined rooms for user")
		return nil
	}
	// send this key change to all servers who share rooms with this user.
	destinations, err := t.db.GetJoinedHostsForRooms(context.Background(), queryRes.RoomIDs)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined hosts for rooms user is in")
		return nil
	}

	// Pack the EDU and marshal it
	edu := &gomatrixserverlib.EDU{
		Type:   gomatrixserverlib.MDeviceListUpdate,
		Origin: string(t.serverName),
	}
	event := gomatrixserverlib.DeviceListUpdateEvent{
		UserID:            m.UserID,
		DeviceID:          m.DeviceID,
		DeviceDisplayName: m.DisplayName,
		StreamID:          m.StreamID,
		PrevID:            prevID(m.StreamID),
		Deleted:           len(m.KeyJSON) == 0,
		Keys:              m.KeyJSON,
	}
	if edu.Content, err = json.Marshal(event); err != nil {
		return err
	}

	log.Infof("Sending device list update message to %q", destinations)
	return t.queues.SendEDU(edu, t.serverName, destinations)
}

func prevID(streamID int) []int {
	if streamID <= 1 {
		return nil
	}
	return []int{streamID - 1}
}
