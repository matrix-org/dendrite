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
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputRoomEvent consumes events that originated in the room server.
type OutputRoomEvent struct {
	roomServerConsumer *common.ContinualConsumer
	db                 *accounts.Database
	query              api.RoomserverQueryAPI
	serverName         string
}

// NewOutputRoomEvent creates a new OutputRoomEvent consumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEvent(cfg *config.Dendrite, store *accounts.Database) (*OutputRoomEvent, error) {
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
		serverName:         string(cfg.Matrix.ServerName),
	}
	consumer.ProcessMessage = s.onMessage

	return s, nil
}

// Start consuming from room servers
func (s *OutputRoomEvent) Start() error {
	return s.roomServerConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
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

	events, err := s.lookupStateEvents(output.NewRoomEvent.AddsStateEventIDs, ev)
	if err != nil {
		return err
	}

	for _, event := range events {
		if err := s.updateMembership(event); err != nil {
			return err
		}
	}

	for _, id := range output.NewRoomEvent.RemovesStateEventIDs {
		if err := s.db.RemoveMembershipByEventID(id); err != nil {
			return err
		}
	}

	return nil
}

// lookupStateEvents looks up the state events that are added by a new event.
func (s *OutputRoomEvent) lookupStateEvents(
	addsStateEventIDs []string, event gomatrixserverlib.Event,
) ([]gomatrixserverlib.Event, error) {
	// Fast path if there aren't any new state events.
	if len(addsStateEventIDs) == 0 {
		return nil, nil
	}

	// Fast path if the only state event added is the event itself.
	if len(addsStateEventIDs) == 1 && addsStateEventIDs[0] == event.EventID() {
		return []gomatrixserverlib.Event{event}, nil
	}

	result := []gomatrixserverlib.Event{}
	missing := []string{}
	for _, id := range addsStateEventIDs {
		// Check if the event is already known
		localpart, server, err := s.db.GetMembershipByEventID(id)
		if err != nil {
			return nil, err
		}

		// Append the ID to the list to request so if it isn't in the database
		if len(localpart) == 0 && len(server) == 0 {
			missing = append(missing, id)
		}

		// Append the current event in the results if its ID is in the events list
		if id == event.EventID() {
			result = append(result, event)
		}
	}

	// At this point the missing events are neither the event itself nor are
	// they present in our local database. Our only option is to fetch them
	// from the roomserver using the query API.
	eventReq := api.QueryEventsByIDRequest{EventIDs: missing}
	var eventResp api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(&eventReq, &eventResp); err != nil {
		return nil, err
	}

	result = append(result, eventResp.Events...)

	return result, nil
}

func (s *OutputRoomEvent) updateMembership(ev gomatrixserverlib.Event) error {
	if ev.Type() == "m.room.member" && ev.StateKey() != nil {
		localpart, serverName, err := gomatrixserverlib.SplitID('@', *ev.StateKey())
		if err != nil {
			return err
		}

		// we only want state events from local users
		if string(serverName) != s.serverName {
			return nil
		}

		eventID := ev.EventID()
		roomID := ev.RoomID()
		membership, err := ev.Membership()
		if err != nil {
			return err
		}

		switch membership {
		case "join":
			if err := s.db.SaveMembership(localpart, roomID, eventID); err != nil {
				return err
			}
		case "leave":
		case "ban":
			if err := s.db.RemoveMembership(localpart, roomID); err != nil {
				return err
			}
		}
	}
	return nil
}
