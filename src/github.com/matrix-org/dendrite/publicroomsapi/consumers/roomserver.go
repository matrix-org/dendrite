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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver/api"
	log "github.com/sirupsen/logrus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputRoomEvent consumes events that originated in the room server.
type OutputRoomEvent struct {
	roomServerConsumer *common.ContinualConsumer
	db                 *storage.PublicRoomsServerDatabase
	query              api.RoomserverQueryAPI
}

// NewOutputRoomEvent creates a new OutputRoomEvent consumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEvent(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	store *storage.PublicRoomsServerDatabase,
	queryAPI api.RoomserverQueryAPI,
) *OutputRoomEvent {
	consumer := common.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputRoomEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEvent{
		roomServerConsumer: &consumer,
		db:                 store,
		query:              queryAPI,
	}
	consumer.ProcessMessage = s.onMessage

	return s
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

	addQueryReq := api.QueryEventsByIDRequest{output.NewRoomEvent.AddsStateEventIDs}
	var addQueryRes api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(&addQueryReq, &addQueryRes); err != nil {
		log.Warn(err)
		return err
	}

	remQueryReq := api.QueryEventsByIDRequest{output.NewRoomEvent.RemovesStateEventIDs}
	var remQueryRes api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(&remQueryReq, &remQueryRes); err != nil {
		log.Warn(err)
		return err
	}

	return s.db.UpdateRoomFromEvents(addQueryRes.Events, remQueryRes.Events)
}
