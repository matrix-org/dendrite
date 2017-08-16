// Copyright 2017 Vector Creations Ltd
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
	"encoding/json"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver/api"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputRoomEvent consumes events that originated in the room server.
type OutputRoomEvent struct {
	roomServerConsumer *common.ContinualConsumer
	db                 *storage.PublicRoomsServerDatabase
	query              api.RoomserverQueryAPI
}

// NewOutputRoomEvent creates a new OutputRoomEvent consumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEvent(cfg *config.Dendrite, store *storage.PublicRoomsServerDatabase) (*OutputRoomEvent, error) {
	kafkaConsumer, err := sarama.NewConsumer(cfg.Kafka.Addresses, nil)
	if err != nil {
		return nil, err
	}
	roomServerURL := cfg.RoomServerURL()

	consumer := common.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputRoomEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEvent{
		roomServerConsumer: &consumer,
		db:                 store,
		query:              api.NewRoomserverQueryAPIHTTP(roomServerURL, nil),
	}
	consumer.ProcessMessage = s.onMessage

	return s, nil
}

// Start consuming from room servers
func (s *OutputRoomEvent) Start() error {
	return s.roomServerConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
func (s *OutputRoomEvent) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output api.OutputEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	if output.Type != api.OutputTypeNewRoomEvent {
		log.WithField("type", output.Type).Debug(
			"roomserver output log: ignoring unknown output type",
		)
		return nil
	}

	ev := output.NewRoomEvent.Event
	log.WithFields(log.Fields{
		"event_id": ev.EventID(),
		"room_id":  ev.RoomID(),
		"type":     ev.Type(),
	}).Info("received event from roomserver")

	return s.db.UpdateRoomFromEvent(ev)
}
