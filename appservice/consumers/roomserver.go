// Copyright 2024 New Vector Ltd.
// Copyright 2018 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/syncapi/synctypes"

	log "github.com/sirupsen/logrus"
)

// ApplicationServiceTransaction is the transaction that is sent off to an
// application service.
type ApplicationServiceTransaction struct {
	Events []synctypes.ClientEvent `json:"events"`
}

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	ctx       context.Context
	cfg       *config.AppServiceAPI
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
	js nats.JetStreamContext,
	rsAPI api.AppserviceRoomserverAPI,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:       process.Context(),
		cfg:       cfg,
		jetstream: js,
		topic:     cfg.Matrix.JetStream.Prefixed(jetstream.OutputAppserviceEvent),
		rsAPI:     rsAPI,
	}
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	durableNames := make([]string, 0, len(s.cfg.Derived.ApplicationServices))
	for _, as := range s.cfg.Derived.ApplicationServices {
		appsvc := as
		state := &appserviceState{
			ApplicationService: &appsvc,
		}
		token := jetstream.Tokenise(as.ID)
		if err := jetstream.JetStreamConsumer(
			s.ctx, s.jetstream, s.topic,
			s.cfg.Matrix.JetStream.Durable("Appservice_"+token),
			50, // maximum number of events to send in a single transaction
			func(ctx context.Context, msgs []*nats.Msg) bool {
				return s.onMessage(ctx, state, msgs)
			},
			nats.DeliverNew(), nats.ManualAck(),
		); err != nil {
			return fmt.Errorf("failed to create %q consumer: %w", token, err)
		}
		durableNames = append(durableNames, s.cfg.Matrix.JetStream.Durable("Appservice_"+token))
	}
	// Cleanup any consumers still existing on the OutputRoomEvent stream
	// to avoid messages not being deleted
	for _, consumerName := range durableNames {
		err := s.jetstream.DeleteConsumer(s.cfg.Matrix.JetStream.Prefixed(jetstream.OutputRoomEvent), consumerName+"Pull")
		if err != nil && err != nats.ErrConsumerNotFound {
			return err
		}
	}
	return nil
}

// onMessage is called when the appservice component receives a new event from
// the room server output log.
func (s *OutputRoomEventConsumer) onMessage(
	ctx context.Context, state *appserviceState, msgs []*nats.Msg,
) bool {
	log.WithField("appservice", state.ID).Tracef("Appservice worker received %d message(s) from roomserver", len(msgs))
	events := make([]*types.HeaderedEvent, 0, len(msgs))
	for _, msg := range msgs {
		// Only handle events we care about
		receivedType := api.OutputType(msg.Header.Get(jetstream.RoomEventType))
		if receivedType != api.OutputTypeNewRoomEvent && receivedType != api.OutputTypeNewInviteEvent {
			continue
		}
		// Parse out the event JSON
		var output api.OutputEvent
		if err := json.Unmarshal(msg.Data, &output); err != nil {
			// If the message was invalid, log it and move on to the next message in the stream
			log.WithField("appservice", state.ID).WithError(err).Errorf("Appservice failed to parse message, ignoring")
			continue
		}
		switch output.Type {
		case api.OutputTypeNewRoomEvent:
			if output.NewRoomEvent == nil || !s.appserviceIsInterestedInEvent(ctx, output.NewRoomEvent.Event, state.ApplicationService) {
				continue
			}
			events = append(events, output.NewRoomEvent.Event)
			if len(output.NewRoomEvent.AddsStateEventIDs) > 0 {
				newEventID := output.NewRoomEvent.Event.EventID()
				eventsReq := &api.QueryEventsByIDRequest{
					RoomID:   output.NewRoomEvent.Event.RoomID().String(),
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

		default:
			continue
		}
	}

	// If there are no events selected for sending then we should
	// ack the messages so that we don't get sent them again in the
	// future.
	if len(events) == 0 {
		return true
	}

	txnID := ""
	// Try to get the message metadata, if we're able to, use the timestamp as the txnID
	metadata, err := msgs[0].Metadata()
	if err == nil {
		txnID = strconv.Itoa(int(metadata.Timestamp.UnixNano()))
	}

	// Send event to any relevant application services. If we hit
	// an error here, return false, so that we negatively ack.
	log.WithField("appservice", state.ID).Debugf("Appservice worker sending %d events(s) from roomserver", len(events))
	return s.sendEvents(ctx, state, events, txnID) == nil
}

// sendEvents passes events to the appservice by using the transactions
// endpoint. It will block for the backoff period if necessary.
func (s *OutputRoomEventConsumer) sendEvents(
	ctx context.Context, state *appserviceState,
	events []*types.HeaderedEvent,
	txnID string,
) error {
	// Create the transaction body.
	transaction, err := json.Marshal(
		ApplicationServiceTransaction{
			Events: synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(events), synctypes.FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
				return s.rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
			}),
		},
	)
	if err != nil {
		return err
	}

	// If txnID is not defined, generate one from the events.
	if txnID == "" {
		txnID = fmt.Sprintf("%d_%d", events[0].PDU.OriginServerTS(), len(transaction))
	}

	// Send the transaction to the appservice.
	// https://spec.matrix.org/v1.9/application-service-api/#pushing-events
	path := "_matrix/app/v1/transactions"
	if s.cfg.LegacyPaths {
		path = "transactions"
	}
	address := fmt.Sprintf("%s/%s/%s", state.RequestUrl(), path, txnID)
	if s.cfg.LegacyAuth {
		address += "?access_token=" + url.QueryEscape(state.HSToken)
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", address, bytes.NewBuffer(transaction))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", state.HSToken))
	resp, err := state.HTTPClient.Do(req)
	if err != nil {
		return state.backoffAndPause(err)
	}

	// If the response was fine then we can clear any backoffs in place and
	// report that everything was OK. Otherwise, back off for a while.
	switch resp.StatusCode {
	case http.StatusOK:
		state.backoff = 0
	default:
		return state.backoffAndPause(fmt.Errorf("received HTTP status code %d from appservice url %s", resp.StatusCode, address))
	}
	return nil
}

// backoff pauses the calling goroutine for a 2^some backoff exponent seconds
func (s *appserviceState) backoffAndPause(err error) error {
	if s.backoff < 6 {
		s.backoff++
	}
	duration := time.Second * time.Duration(math.Pow(2, float64(s.backoff)))
	log.WithField("appservice", s.ID).WithError(err).Errorf("Unable to send transaction to appservice, backing off for %s", duration.String())
	time.Sleep(duration)
	return err
}

// appserviceIsInterestedInEvent returns a boolean depending on whether a given
// event falls within one of a given application service's namespaces.
//
// TODO: This should be cached, see https://github.com/element-hq/dendrite/issues/1682
func (s *OutputRoomEventConsumer) appserviceIsInterestedInEvent(ctx context.Context, event *types.HeaderedEvent, appservice *config.ApplicationService) bool {
	user := ""
	userID, err := s.rsAPI.QueryUserIDForSender(ctx, event.RoomID(), event.SenderID())
	if err == nil {
		user = userID.String()
	}

	switch {
	case appservice.URL == "":
		return false
	case appservice.IsInterestedInUserID(user):
		return true
	case appservice.IsInterestedInRoomID(event.RoomID().String()):
		return true
	}

	if event.Type() == spec.MRoomMember && event.StateKey() != nil {
		if appservice.IsInterestedInUserID(*event.StateKey()) {
			return true
		}
	}

	// Check all known room aliases of the room the event came from
	queryReq := api.GetAliasesForRoomIDRequest{RoomID: event.RoomID().String()}
	var queryRes api.GetAliasesForRoomIDResponse
	if err := s.rsAPI.GetAliasesForRoomID(ctx, &queryReq, &queryRes); err == nil {
		for _, alias := range queryRes.Aliases {
			if appservice.IsInterestedInRoomAlias(alias) {
				return true
			}
		}
	} else {
		log.WithFields(log.Fields{
			"appservice": appservice.ID,
			"room_id":    event.RoomID().String(),
		}).WithError(err).Errorf("Unable to get aliases for room")
	}

	// Check if any of the members in the room match the appservice
	return s.appserviceJoinedAtEvent(ctx, event, appservice)
}

// appserviceJoinedAtEvent returns a boolean depending on whether a given
// appservice has membership at the time a given event was created.
func (s *OutputRoomEventConsumer) appserviceJoinedAtEvent(ctx context.Context, event *types.HeaderedEvent, appservice *config.ApplicationService) bool {
	// TODO: This is only checking the current room state, not the state at
	// the event in question. Pretty sure this is what Synapse does too, but
	// until we have a lighter way of checking the state before the event that
	// doesn't involve state res, then this is probably OK.
	membershipReq := &api.QueryMembershipsForRoomRequest{
		RoomID:     event.RoomID().String(),
		JoinedOnly: true,
	}
	membershipRes := &api.QueryMembershipsForRoomResponse{}

	// XXX: This could potentially race if the state for the event is not known yet
	// e.g. the event came over federation but we do not have the full state persisted.
	if err := s.rsAPI.QueryMembershipsForRoom(ctx, membershipReq, membershipRes); err == nil {
		for _, ev := range membershipRes.JoinEvents {
			switch {
			case ev.StateKey == nil:
				continue
			case ev.Type != spec.MRoomMember:
				continue
			}
			var membership gomatrixserverlib.MemberContent
			err = json.Unmarshal(ev.Content, &membership)
			switch {
			case err != nil:
				continue
			case membership.Membership == spec.Join:
				if appservice.IsInterestedInUserID(*ev.StateKey) {
					return true
				}
			}
		}
	} else {
		log.WithFields(log.Fields{
			"appservice": appservice.ID,
			"room_id":    event.RoomID().String(),
		}).WithError(err).Errorf("Unable to get membership for room")
	}
	return false
}
