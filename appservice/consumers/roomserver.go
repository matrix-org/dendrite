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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"

	log "github.com/sirupsen/logrus"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	ctx       context.Context
	cfg       *config.AppServiceAPI
	client    *http.Client
	jetstream nats.JetStreamContext
	topic     string
	rsAPI     api.AppserviceRoomserverAPI
}

type appserviceState struct {
	*config.ApplicationService
	backoff int
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call
// Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.AppServiceAPI,
	client *http.Client,
	js nats.JetStreamContext,
	rsAPI api.AppserviceRoomserverAPI,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:       process.Context(),
		cfg:       cfg,
		client:    client,
		jetstream: js,
		topic:     cfg.Matrix.JetStream.Prefixed(jetstream.OutputRoomEvent),
		rsAPI:     rsAPI,
	}
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	for _, as := range s.cfg.Derived.ApplicationServices {
		appsvc := as
		state := &appserviceState{
			ApplicationService: &appsvc,
		}
		token := jetstream.Tokenise(as.ID)
		if err := jetstream.JetStreamConsumer(
			s.ctx, s.jetstream, s.topic,
			s.cfg.Matrix.JetStream.Durable("Appservice_"+token), 50,
			func(ctx context.Context, msgs []*nats.Msg) bool {
				return s.onMessage(ctx, state, msgs)
			},
			nats.DeliverAll(), nats.ManualAck(),
		); err != nil {
			return fmt.Errorf("failed to create %q consumer: %w", token, err)
		}
	}
	return nil
}

// onMessage is called when the appservice component receives a new event from
// the room server output log.
func (s *OutputRoomEventConsumer) onMessage(
	ctx context.Context, state *appserviceState, msgs []*nats.Msg,
) bool {
	log.WithField("appservice", state.ID).Debugf("Appservice worker received %d message(s) from roomserver", len(msgs))
	events := make([]*gomatrixserverlib.HeaderedEvent, 0, len(msgs))
	for _, msg := range msgs {
		// Parse out the event JSON
		var output api.OutputEvent
		if err := json.Unmarshal(msg.Data, &output); err != nil {
			// If the message was invalid, log it and move on to the next message in the stream
			log.WithField("appservice", state.ID).WithError(err).Errorf("Appservice failed to parse message, ignoring")
			continue
		}
		switch output.Type {
		case api.OutputTypeNewRoomEvent:
			if output.NewRoomEvent == nil {
				continue
			}
			events = append(events, output.NewRoomEvent.Event)
			if len(output.NewRoomEvent.AddsStateEventIDs) > 0 {
				newEventID := output.NewRoomEvent.Event.EventID()
				eventsReq := &api.QueryEventsByIDRequest{
					EventIDs: make([]string, 0, len(output.NewRoomEvent.AddsStateEventIDs)),
				}
				eventsRes := &api.QueryEventsByIDResponse{}
				for _, eventID := range output.NewRoomEvent.AddsStateEventIDs {
					if eventID != newEventID {
						eventsReq.EventIDs = append(eventsReq.EventIDs, eventID)
					}
				}
				if len(eventsReq.EventIDs) > 0 {
					if err := s.rsAPI.QueryEventsByID(s.ctx, eventsReq, eventsRes); err != nil {
						log.WithError(err).Errorf("s.rsAPI.QueryEventsByID failed")
						return false
					}
					events = append(events, eventsRes.Events...)
				}
			}

		case api.OutputTypeNewInviteEvent:
			if output.NewInviteEvent == nil {
				continue
			}
			events = append(events, output.NewInviteEvent.Event)

		default:
			continue
		}
	}

	// Send event to any relevant application services
	if err := s.filterRoomserverEvents(ctx, state, events); err != nil {
		log.WithField("appservice", state.ID).WithError(err).Errorf("Appservice failed to filter events")
		return false
	}

	return true
}

// filterRoomserverEvents takes in events and decides whether any of them need
// to be passed on to an external application service. It does this by checking
// each namespace of each registered application service, and if there is a
// match, adds the event to the queue for events to be sent to a particular
// application service.
func (s *OutputRoomEventConsumer) filterRoomserverEvents(
	ctx context.Context, state *appserviceState,
	events []*gomatrixserverlib.HeaderedEvent,
) error {
	// Filter out the events down to only ones that the appservice has
	// any interest in.
	// TODO: We can probably benefit from some caching here somewhere.
	filteredEvents := make([]*gomatrixserverlib.HeaderedEvent, 0, len(events))
	for _, event := range events {
		if s.appserviceIsInterestedInEvent(ctx, event, state.ApplicationService) {
			continue
		}
		filteredEvents = append(filteredEvents, event)
	}

	// If there are no events that we are interested in then don't bother
	// doing anything else at this point.
	if len(filteredEvents) == 0 {
		return nil
	}

	// Create the transaction body.
	transaction, err := json.Marshal(gomatrixserverlib.ApplicationServiceTransaction{
		Events: gomatrixserverlib.HeaderedToClientEvents(filteredEvents, gomatrixserverlib.FormatAll),
	})
	if err != nil {
		return err
	}

	// TODO: We should probably be more intelligent and pick something not
	// in the control of the event. A NATS timestamp header or something maybe.
	txnID := filteredEvents[0].Event.OriginServerTS()

	// Send the transaction to the appservice.
	// https://matrix.org/docs/spec/application_service/r0.1.2#put-matrix-app-v1-transactions-txnid
	address := fmt.Sprintf("%s/transactions/%d?access_token=%s", state.URL, txnID, url.QueryEscape(state.HSToken))
	req, err := http.NewRequest("PUT", address, bytes.NewBuffer(transaction))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		state.backoffAndPause(err)
		return err
	}

	// If the response was fine then we can clear any backoffs in place and
	// report that everything was OK. Otherwise, back off for a while.
	switch resp.StatusCode {
	case http.StatusOK:
		state.backoff = 0
	default:
		state.backoffAndPause(err)
	}
	return nil
}

// backoff pauses the calling goroutine for a 2^some backoff exponent seconds
func (s *appserviceState) backoffAndPause(err error) {
	if s.backoff < 6 {
		s.backoff++
	}

	duration := time.Second * time.Duration(math.Pow(2, float64(s.backoff)))
	log.WithField("appservice", s.ID).WithError(err).Warnf("Unable to send transaction to appservice successfully, backing off for %s", duration.String())
	time.Sleep(duration)
}

// appserviceIsInterestedInEvent returns a boolean depending on whether a given
// event falls within one of a given application service's namespaces.
//
// TODO: This should be cached, see https://github.com/matrix-org/dendrite/issues/1682
func (s *OutputRoomEventConsumer) appserviceIsInterestedInEvent(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, appservice *config.ApplicationService) bool {
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

// appserviceJoinedAtEvent returns a boolean depending on whether a given
// appservice has membership at the time a given event was created.
func (s *OutputRoomEventConsumer) appserviceJoinedAtEvent(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, appservice *config.ApplicationService) bool {
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
