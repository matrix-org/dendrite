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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/sirupsen/logrus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	roomServerConsumer *common.ContinualConsumer
	db                 accounts.Database
	query              api.RoomserverQueryAPI
	serverName         string
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	store accounts.Database,
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
		serverName:         string(cfg.Matrix.ServerName),
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return s.roomServerConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
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

	log.WithFields(log.Fields{
		"event_id": output.NewRoomEvent.Event.EventID(),
		"room_id":  output.NewRoomEvent.Event.RoomID(),
		"type":     output.NewRoomEvent.Event.Type(),
	}).Info("received event from roomserver")

	events, err := s.lookupStateEvents(output.NewRoomEvent.AddsStateEventIDs, output.NewRoomEvent.Event)
	if err != nil {
		return err
	}

	return s.db.UpdateMemberships(context.TODO(), events, output.NewRoomEvent.RemovesStateEventIDs)
}

// lookupStateEvents looks up the state events that are added by a new event.
func (s *OutputRoomEventConsumer) lookupStateEvents(
	addsStateEventIDs []string, event gomatrixserverlib.Event,
) ([]gomatrixserverlib.Event, error) {
	// Fast path if there aren't any new state events.
	if len(addsStateEventIDs) == 0 {
		// If the event is a membership update (e.g. for a profile update), it won't
		// show up in AddsStateEventIDs, so we need to add it manually
		if event.Type() == "m.room.member" {
			return []gomatrixserverlib.Event{event}, nil
		}
		return nil, nil
	}

	// Fast path if the only state event added is the event itself.
	if len(addsStateEventIDs) == 1 && addsStateEventIDs[0] == event.EventID() {
		return []gomatrixserverlib.Event{event}, nil
	}

	result := []gomatrixserverlib.Event{}
	missing := []string{}
	for _, id := range addsStateEventIDs {
		// Append the current event in the results if its ID is in the events list
		if id == event.EventID() {
			result = append(result, event)
		} else {
			// If the event isn't the current one, add it to the list of events
			// to retrieve from the roomserver
			missing = append(missing, id)
		}
	}

	// Request the missing events from the roomserver
	eventReq := api.QueryEventsByIDRequest{EventIDs: missing}
	var eventResp api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(context.TODO(), &eventReq, &eventResp); err != nil {
		return nil, err
	}

	result = append(result, eventResp.Events...)

	return result, nil
}
