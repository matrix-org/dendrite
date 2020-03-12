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
	"context"
	"encoding/json"
	"errors"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	roomServerConsumer *common.ContinualConsumer
	db                 storage.Database
	query              api.RoomserverQueryAPI
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
	queryAPI api.RoomserverQueryAPI,
) *OutputRoomEventConsumer {
	consumer := common.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputRoomEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEventConsumer{
		roomServerConsumer: &consumer,
		db:                 store,
		query:              queryAPI,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return s.roomServerConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output api.OutputEvent
	headers := common.SaramaHeaders(msg.Headers)

	if msgtype, ok := headers["type"]; ok {
		if api.OutputType(msgtype) != api.OutputTypeNewRoomEvent {
			log.WithField("type", msgtype).Debug(
				"roomserver output log: ignoring unknown output type",
			)
			return nil
		}
	} else {
		log.WithField("type", msgtype).Debug(
			"roomserver output log: no message type included",
		)
		return nil
	}

	// See if the room version is present in the headers. If it isn't
	// then we can't process the event as we don't know what the format
	// will be
	var roomVersion gomatrixserverlib.RoomVersion
	if rv, ok := headers["room_version"]; ok {
		roomVersion = gomatrixserverlib.RoomVersion(rv)
	}
	if roomVersion == "" {
		return errors.New("room version was not in sarama headers")
	}

	// Prepare the room event so that it has the correct field types
	// for the room version
	output.NewRoomEvent = &api.OutputNewRoomEvent{
		Event: gomatrixserverlib.Event{},
	}
	if err := output.NewRoomEvent.Event.PrepareAs(roomVersion); err != nil {
		log.WithFields(log.Fields{
			"room_version": roomVersion,
		}).WithError(err).Errorf("can't prepare event to version")
		return err
	}

	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	ev := output.NewRoomEvent.Event
	log.WithFields(log.Fields{
		"event_id": ev.EventID(),
		"room_id":  ev.RoomID(),
		"type":     ev.Type(),
	}).Info("received event from roomserver")

	addQueryReq := api.QueryEventsByIDRequest{EventIDs: output.NewRoomEvent.AddsStateEventIDs}
	var addQueryRes api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(context.TODO(), &addQueryReq, &addQueryRes); err != nil {
		log.Warn(err)
		return err
	}

	remQueryReq := api.QueryEventsByIDRequest{EventIDs: output.NewRoomEvent.RemovesStateEventIDs}
	var remQueryRes api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(context.TODO(), &remQueryReq, &remQueryRes); err != nil {
		log.Warn(err)
		return err
	}

	return s.db.UpdateRoomFromEvents(context.TODO(), addQueryRes.Events, remQueryRes.Events)
}
