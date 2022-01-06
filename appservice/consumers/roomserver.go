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
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"

	log "github.com/sirupsen/logrus"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	ctx          context.Context
	jetstream    nats.JetStreamContext
	topic        string
	asDB         storage.Database
	rsAPI        api.RoomserverInternalAPI
	serverName   string
	workerStates []types.ApplicationServiceWorkerState
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call
// Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.Dendrite,
	js nats.JetStreamContext,
	appserviceDB storage.Database,
	rsAPI api.RoomserverInternalAPI,
	workerStates []types.ApplicationServiceWorkerState,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:          process.Context(),
		jetstream:    js,
		topic:        cfg.Global.JetStream.TopicFor(jetstream.OutputRoomEvent),
		asDB:         appserviceDB,
		rsAPI:        rsAPI,
		serverName:   string(cfg.Global.ServerName),
		workerStates: workerStates,
	}
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	_, err := s.jetstream.Subscribe(s.topic, s.onMessage)
	return err
}

// onMessage is called when the appservice component receives a new event from
// the room server output log.
func (s *OutputRoomEventConsumer) onMessage(msg *nats.Msg) {
	jetstream.WithJetStreamMessage(msg, func(msg *nats.Msg) bool {
		// Parse out the event JSON
		var output api.OutputEvent
		if err := json.Unmarshal(msg.Data, &output); err != nil {
			// If the message was invalid, log it and move on to the next message in the stream
			log.WithError(err).Errorf("roomserver output log: message parse failure")
			return true
		}

		if output.Type != api.OutputTypeNewRoomEvent {
			return true
		}

		events := []*gomatrixserverlib.HeaderedEvent{output.NewRoomEvent.Event}
		events = append(events, output.NewRoomEvent.AddStateEvents...)

		// Send event to any relevant application services
		if err := s.filterRoomserverEvents(context.TODO(), events); err != nil {
			log.WithError(err).Errorf("roomserver output log: filter error")
			return true
		}

		return true
	})
}

// filterRoomserverEvents takes in events and decides whether any of them need
// to be passed on to an external application service. It does this by checking
// each namespace of each registered application service, and if there is a
// match, adds the event to the queue for events to be sent to a particular
// application service.
func (s *OutputRoomEventConsumer) filterRoomserverEvents(
	ctx context.Context,
	events []*gomatrixserverlib.HeaderedEvent,
) error {
	for _, ws := range s.workerStates {
		for _, event := range events {
			// Check if this event is interesting to this application service
			if s.appserviceIsInterestedInEvent(ctx, event, ws.AppService) {
				// Queue this event to be sent off to the application service
				if err := s.asDB.StoreEvent(ctx, ws.AppService.ID, event); err != nil {
					log.WithError(err).Warn("failed to insert incoming event into appservices database")
					return err
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

// appserviceJoinedAtEvent returns a boolean depending on whether a given
// appservice has membership at the time a given event was created.
func (s *OutputRoomEventConsumer) appserviceJoinedAtEvent(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, appservice config.ApplicationService) bool {
	// TODO: This is only checking the current room state, not the state at
	// the event in question. Pretty sure this is what Synapse does too, but
	// until we have a lighter way of checking the state before the event that
	// doesn't involve state res, then this is probably OK.
	membershipReq := &api.QueryMembershipsForRoomRequest{
		RoomID:     event.RoomID(),
		JoinedOnly: true,
	}
	membershipRes := &api.QueryMembershipsForRoomResponse{}

	// XXX: This could potentially race if the state for the event is not known yet
	// e.g. the event came over federation but we do not have the full state persisted.
	if err := s.rsAPI.QueryMembershipsForRoom(ctx, membershipReq, membershipRes); err == nil {
		for _, ev := range membershipRes.JoinEvents {
			var membership gomatrixserverlib.MemberContent
			if err = json.Unmarshal(ev.Content, &membership); err != nil || ev.StateKey == nil {
				continue
			}
			if appservice.IsInterestedInUserID(*ev.StateKey) {
				return true
			}
		}
	} else {
		log.WithFields(log.Fields{
			"room_id": event.RoomID(),
		}).WithError(err).Errorf("Unable to get membership for room")
	}
	return false
}

// appserviceIsInterestedInEvent returns a boolean depending on whether a given
// event falls within one of a given application service's namespaces.
//
// TODO: This should be cached, see https://github.com/matrix-org/dendrite/issues/1682
func (s *OutputRoomEventConsumer) appserviceIsInterestedInEvent(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, appservice config.ApplicationService) bool {
	// No reason to queue events if they'll never be sent to the application
	// service
	if appservice.URL == "" {
		return false
	}

	// Check Room ID and Sender of the event
	if appservice.IsInterestedInUserID(event.Sender()) ||
		appservice.IsInterestedInRoomID(event.RoomID()) {
		return true
	}

	if event.Type() == gomatrixserverlib.MRoomMember && event.StateKey() != nil {
		if appservice.IsInterestedInUserID(*event.StateKey()) {
			return true
		}
	}

	// Check all known room aliases of the room the event came from
	queryReq := api.GetAliasesForRoomIDRequest{RoomID: event.RoomID()}
	var queryRes api.GetAliasesForRoomIDResponse
	if err := s.rsAPI.GetAliasesForRoomID(ctx, &queryReq, &queryRes); err == nil {
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

	// Check if any of the members in the room match the appservice
	return s.appserviceJoinedAtEvent(ctx, event, appservice)
}
