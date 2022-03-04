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
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/producers"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	ctx          context.Context
	cfg          *config.SyncAPI
	rsAPI        api.RoomserverInternalAPI
	jetstream    nats.JetStreamContext
	durable      string
	topic        string
	db           storage.Database
	pduStream    types.StreamProvider
	inviteStream types.StreamProvider
	notifier     *notifier.Notifier
	producer     *producers.UserAPIStreamEventProducer
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	notifier *notifier.Notifier,
	pduStream types.StreamProvider,
	inviteStream types.StreamProvider,
	rsAPI api.RoomserverInternalAPI,
	producer *producers.UserAPIStreamEventProducer,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:          process.Context(),
		cfg:          cfg,
		jetstream:    js,
		topic:        cfg.Matrix.JetStream.TopicFor(jetstream.OutputRoomEvent),
		durable:      cfg.Matrix.JetStream.Durable("SyncAPIRoomServerConsumer"),
		db:           store,
		notifier:     notifier,
		pduStream:    pduStream,
		inviteStream: inviteStream,
		rsAPI:        rsAPI,
		producer:     producer,
	}
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputRoomEventConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	// Parse out the event JSON
	var err error
	var output api.OutputEvent
	if err = json.Unmarshal(msg.Data, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return true
	}

	switch output.Type {
	case api.OutputTypeNewRoomEvent:
		// Ignore redaction events. We will add them to the database when they are
		// validated (when we receive OutputTypeRedactedEvent)
		event := output.NewRoomEvent.Event
		if event.Type() == gomatrixserverlib.MRoomRedaction && event.StateKey() == nil {
			// in the special case where the event redacts itself, just pass the message through because
			// we will never see the other part of the pair
			if event.Redacts() != event.EventID() {
				return true
			}
		}
		err = s.onNewRoomEvent(s.ctx, *output.NewRoomEvent)
	case api.OutputTypeOldRoomEvent:
		err = s.onOldRoomEvent(s.ctx, *output.OldRoomEvent)
	case api.OutputTypeNewInviteEvent:
		s.onNewInviteEvent(s.ctx, *output.NewInviteEvent)
	case api.OutputTypeRetireInviteEvent:
		s.onRetireInviteEvent(s.ctx, *output.RetireInviteEvent)
	case api.OutputTypeNewPeek:
		s.onNewPeek(s.ctx, *output.NewPeek)
	case api.OutputTypeRetirePeek:
		s.onRetirePeek(s.ctx, *output.RetirePeek)
	case api.OutputTypeRedactedEvent:
		err = s.onRedactEvent(s.ctx, *output.RedactedEvent)
	default:
		log.WithField("type", output.Type).Debug(
			"roomserver output log: ignoring unknown output type",
		)
	}
	if err != nil {
		log.WithError(err).Error("roomserver output log: failed to process event")
		return false
	}

	return true
}

func (s *OutputRoomEventConsumer) onRedactEvent(
	ctx context.Context, msg api.OutputRedactedEvent,
) error {
	err := s.db.RedactEvent(ctx, msg.RedactedEventID, msg.RedactedBecause)
	if err != nil {
		log.WithError(err).Error("RedactEvent error'd")
		return err
	}
	// fake a room event so we notify clients about the redaction, as if it were
	// a normal event.
	return s.onNewRoomEvent(ctx, api.OutputNewRoomEvent{
		Event: msg.RedactedBecause,
	})
}

func (s *OutputRoomEventConsumer) onNewRoomEvent(
	ctx context.Context, msg api.OutputNewRoomEvent,
) error {
	ev := msg.Event
	addsStateEvents := msg.AddsState()

	ev, err := s.updateStateEvent(ev)
	if err != nil {
		sentry.CaptureException(err)
		return err
	}

	for i := range addsStateEvents {
		addsStateEvents[i], err = s.updateStateEvent(addsStateEvents[i])
		if err != nil {
			sentry.CaptureException(err)
			return err
		}
	}

	if msg.RewritesState {
		if err = s.db.PurgeRoomState(ctx, ev.RoomID()); err != nil {
			sentry.CaptureException(err)
			return fmt.Errorf("s.db.PurgeRoom: %w", err)
		}
	}

	pduPos, err := s.db.WriteEvent(
		ctx,
		ev,
		addsStateEvents,
		msg.AddsStateEventIDs,
		msg.RemovesStateEventIDs,
		msg.TransactionID,
		false,
	)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   ev.EventID(),
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
			"add":        msg.AddsStateEventIDs,
			"del":        msg.RemovesStateEventIDs,
		}).Panicf("roomserver output log: write new event failure")
		return nil
	}

	if err = s.producer.SendStreamEvent(ev.RoomID(), ev, pduPos); err != nil {
		log.WithError(err).Errorf("Failed to send stream output event for event %s", ev.EventID())
		sentry.CaptureException(err)
		return err
	}

	if pduPos, err = s.notifyJoinedPeeks(ctx, ev, pduPos); err != nil {
		log.WithError(err).Errorf("Failed to notifyJoinedPeeks for PDU pos %d", pduPos)
		sentry.CaptureException(err)
		return err
	}

	s.pduStream.Advance(pduPos)
	s.notifier.OnNewEvent(ev, ev.RoomID(), nil, types.StreamingToken{PDUPosition: pduPos})

	return nil
}

func (s *OutputRoomEventConsumer) onOldRoomEvent(
	ctx context.Context, msg api.OutputOldRoomEvent,
) error {
	ev := msg.Event

	// TODO: The state key check when excluding from sync is designed
	// to stop us from lying to clients with old state, whilst still
	// allowing normal timeline events through. This is an absolute
	// hack but until we have some better strategy for dealing with
	// old events in the sync API, this should at least prevent us
	// from confusing clients into thinking they've joined/left rooms.
	pduPos, err := s.db.WriteEvent(
		ctx,
		ev,
		[]*gomatrixserverlib.HeaderedEvent{},
		[]string{},           // adds no state
		[]string{},           // removes no state
		nil,                  // no transaction
		ev.StateKey() != nil, // exclude from sync?
	)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   ev.EventID(),
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write old event failure")
		return nil
	}

	if pduPos, err = s.notifyJoinedPeeks(ctx, ev, pduPos); err != nil {
		log.WithError(err).Errorf("Failed to notifyJoinedPeeks for PDU pos %d", pduPos)
		return err
	}

	s.pduStream.Advance(pduPos)
	s.notifier.OnNewEvent(ev, ev.RoomID(), nil, types.StreamingToken{PDUPosition: pduPos})

	return nil
}

func (s *OutputRoomEventConsumer) notifyJoinedPeeks(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent, sp types.StreamPosition) (types.StreamPosition, error) {
	if ev.Type() != gomatrixserverlib.MRoomMember {
		return sp, nil
	}
	membership, err := ev.Membership()
	if err != nil {
		return sp, fmt.Errorf("ev.Membership: %w", err)
	}
	// TODO: check that it's a join and not a profile change (means unmarshalling prev_content)
	if membership == gomatrixserverlib.Join {
		// check it's a local join
		_, domain, err := gomatrixserverlib.SplitID('@', *ev.StateKey())
		if err != nil {
			return sp, fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
		}
		if domain != s.cfg.Matrix.ServerName {
			return sp, nil
		}

		// cancel any peeks for it
		peekSP, peekErr := s.db.DeletePeeks(ctx, ev.RoomID(), *ev.StateKey())
		if peekErr != nil {
			return sp, fmt.Errorf("s.db.DeletePeeks: %w", peekErr)
		}
		if peekSP > 0 {
			sp = peekSP
		}
	}
	return sp, nil
}

func (s *OutputRoomEventConsumer) onNewInviteEvent(
	ctx context.Context, msg api.OutputNewInviteEvent,
) {
	if msg.Event.StateKey() == nil {
		log.WithFields(log.Fields{
			"event": string(msg.Event.JSON()),
		}).Panicf("roomserver output log: invite has no state key")
		return
	}
	pduPos, err := s.db.AddInviteEvent(ctx, msg.Event)
	if err != nil {
		sentry.CaptureException(err)
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   msg.Event.EventID(),
			"event":      string(msg.Event.JSON()),
			"pdupos":     pduPos,
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write invite failure")
		return
	}

	s.inviteStream.Advance(pduPos)
	s.notifier.OnNewInvite(types.StreamingToken{InvitePosition: pduPos}, *msg.Event.StateKey())
}

func (s *OutputRoomEventConsumer) onRetireInviteEvent(
	ctx context.Context, msg api.OutputRetireInviteEvent,
) {
	pduPos, err := s.db.RetireInviteEvent(ctx, msg.EventID)
	// It's possible we just haven't heard of this invite yet, so
	// we should not panic if we try to retire it.
	if err != nil && err != sql.ErrNoRows {
		sentry.CaptureException(err)
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   msg.EventID,
			log.ErrorKey: err,
		}).Panicf("roomserver output log: remove invite failure")
		return
	}

	// Notify any active sync requests that the invite has been retired.
	s.inviteStream.Advance(pduPos)
	s.notifier.OnNewInvite(types.StreamingToken{InvitePosition: pduPos}, msg.TargetUserID)
}

func (s *OutputRoomEventConsumer) onNewPeek(
	ctx context.Context, msg api.OutputNewPeek,
) {
	sp, err := s.db.AddPeek(ctx, msg.RoomID, msg.UserID, msg.DeviceID)
	if err != nil {
		sentry.CaptureException(err)
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write peek failure")
		return
	}

	// tell the notifier about the new peek so it knows to wake up new devices
	// TODO: This only works because the peeks table is reusing the same
	// index as PDUs, but we should fix this
	s.pduStream.Advance(sp)
	s.notifier.OnNewPeek(msg.RoomID, msg.UserID, msg.DeviceID, types.StreamingToken{PDUPosition: sp})
}

func (s *OutputRoomEventConsumer) onRetirePeek(
	ctx context.Context, msg api.OutputRetirePeek,
) {
	sp, err := s.db.DeletePeek(ctx, msg.RoomID, msg.UserID, msg.DeviceID)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write peek failure")
		return
	}

	// tell the notifier about the new peek so it knows to wake up new devices
	// TODO: This only works because the peeks table is reusing the same
	// index as PDUs, but we should fix this
	s.pduStream.Advance(sp)
	s.notifier.OnRetirePeek(msg.RoomID, msg.UserID, msg.DeviceID, types.StreamingToken{PDUPosition: sp})
}

func (s *OutputRoomEventConsumer) updateStateEvent(event *gomatrixserverlib.HeaderedEvent) (*gomatrixserverlib.HeaderedEvent, error) {
	if event.StateKey() == nil {
		return event, nil
	}
	stateKey := *event.StateKey()

	prevEvent, err := s.db.GetStateEvent(
		context.TODO(), event.RoomID(), event.Type(), stateKey,
	)
	if err != nil {
		return event, err
	}

	if prevEvent == nil || prevEvent.EventID() == event.EventID() {
		return event, nil
	}

	prev := types.PrevEventRef{
		PrevContent:   prevEvent.Content(),
		ReplacesState: prevEvent.EventID(),
		PrevSender:    prevEvent.Sender(),
	}

	event.Event, err = event.SetUnsigned(prev)
	return event, err
}
