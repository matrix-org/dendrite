// Copyright 2018 Vector Creations Ltd
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

	"github.com/matrix-org/dendrite/appservice/storage"
	"github.com/matrix-org/dendrite/appservice/types"
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
	db                 *accounts.Database
	asDB               *storage.Database
	query              api.RoomserverQueryAPI
	alias              api.RoomserverAliasAPI
	serverName         string
	workerStates       []types.ApplicationServiceWorkerState
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call
// Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	store *accounts.Database,
	appserviceDB *storage.Database,
	queryAPI api.RoomserverQueryAPI,
	aliasAPI api.RoomserverAliasAPI,
	workerStates []types.ApplicationServiceWorkerState,
) *OutputRoomEventConsumer {
	consumer := common.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputRoomEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEventConsumer{
		roomServerConsumer: &consumer,
		db:                 store,
		asDB:               appserviceDB,
		query:              queryAPI,
		alias:              aliasAPI,
		serverName:         string(cfg.Matrix.ServerName),
		workerStates:       workerStates,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return s.roomServerConsumer.Start()
}

// onMessage is called when the appservice component receives a new event from
// the room server output log.
func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
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
	}).Info("appservice received an event from roomserver")

	missingEvents, err := s.lookupMissingStateEvents(output.NewRoomEvent.AddsStateEventIDs, ev)
	if err != nil {
		return err
	}
	events := append(missingEvents, ev)

	// Send event to any relevant application services
	return s.filterRoomserverEvents(context.TODO(), events)
}

// lookupMissingStateEvents looks up the state events that are added by a new event,
// and returns any not already present.
func (s *OutputRoomEventConsumer) lookupMissingStateEvents(
	addsStateEventIDs []string, event gomatrixserverlib.Event,
) ([]gomatrixserverlib.Event, error) {
	// Fast path if there aren't any new state events.
	if len(addsStateEventIDs) == 0 {
		return []gomatrixserverlib.Event{}, nil
	}

	// Fast path if the only state event added is the event itself.
	if len(addsStateEventIDs) == 1 && addsStateEventIDs[0] == event.EventID() {
		return []gomatrixserverlib.Event{}, nil
	}

	result := []gomatrixserverlib.Event{}
	missing := []string{}
	for _, id := range addsStateEventIDs {
		if id != event.EventID() {
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

// filterRoomserverEvents takes in events and decides whether any of them need
// to be passed on to an external application service. It does this by checking
// each namespace of each registered application service, and if there is a
// match, adds the event to the queue for events to be sent to a particular
// application service.
func (s *OutputRoomEventConsumer) filterRoomserverEvents(
	ctx context.Context,
	events []gomatrixserverlib.Event,
) error {
	for _, ws := range s.workerStates {
		for _, event := range events {
			// Check if this event is interesting to this application service
			if s.appserviceIsInterestedInEvent(ctx, event, ws.AppService) {
				// Queue this event to be sent off to the application service
				if err := s.asDB.StoreEvent(ctx, ws.AppService.ID, &event); err != nil {
					log.WithError(err).Warn("failed to insert incoming event into appservices database")
				} else {
					// Tell our worker to send out new messages by updating remaining message
					// count and waking them up with a broadcast
					ws.NotifyNewEvents()
				}
			}
		}
	}

	return nil
}

// appserviceIsInterestedInEvent returns a boolean depending on whether a given
// event falls within one of a given application service's namespaces.
func (s *OutputRoomEventConsumer) appserviceIsInterestedInEvent(ctx context.Context, event gomatrixserverlib.Event, appservice config.ApplicationService) bool {
	// No reason to queue events if they'll never be sent to the application
	// service
	if appservice.URL == "" {
		return false
	}

	if appservice.IsInterestedInUserID(event.Sender()) ||
		appservice.IsInterestedInRoomID(event.RoomID()) {
		return true
	}

	// Check all known room aliases of the room the event came from
	queryReq := api.GetAliasesForRoomIDRequest{RoomID: event.RoomID()}
	var queryRes api.GetAliasesForRoomIDResponse
	if err := s.alias.GetAliasesForRoomID(ctx, &queryReq, &queryRes); err == nil {
		for _, alias := range queryRes.Aliases {
			if appservice.IsInterestedInRoomAlias(alias) {
				return true
			}
		}
	} else {
		log.WithFields(log.Fields{
			"room_id": event.RoomID(),
		}).WithError(err).Errorf("Unable to get aliases for room")
	}

	return false
}
