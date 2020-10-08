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
	"fmt"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	cfg        *config.SyncAPI
	rsAPI      api.RoomserverInternalAPI
	rsConsumer *internal.ContinualConsumer
	db         storage.Database
	notifier   *sync.Notifier
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	cfg *config.SyncAPI,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store storage.Database,
	rsAPI api.RoomserverInternalAPI,
) *OutputRoomEventConsumer {

	consumer := internal.ContinualConsumer{
		ComponentName:  "syncapi/roomserver",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputRoomEvent)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEventConsumer{
		cfg:        cfg,
		rsConsumer: &consumer,
		db:         store,
		notifier:   n,
		rsAPI:      rsAPI,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return s.rsConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output api.OutputEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
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
				return nil
			}
		}
		return s.onNewRoomEvent(context.TODO(), *output.NewRoomEvent)
	case api.OutputTypeNewInviteEvent:
		return s.onNewInviteEvent(context.TODO(), *output.NewInviteEvent)
	case api.OutputTypeRetireInviteEvent:
		return s.onRetireInviteEvent(context.TODO(), *output.RetireInviteEvent)
	case api.OutputTypeNewPeek:
		return s.onNewPeek(context.TODO(), *output.NewPeek)
	case api.OutputTypeRedactedEvent:
		return s.onRedactEvent(context.TODO(), *output.RedactedEvent)
	default:
		log.WithField("type", output.Type).Debug(
			"roomserver output log: ignoring unknown output type",
		)
		return nil
	}
}

func (s *OutputRoomEventConsumer) onRedactEvent(
	ctx context.Context, msg api.OutputRedactedEvent,
) error {
	err := s.db.RedactEvent(ctx, msg.RedactedEventID, &msg.RedactedBecause)
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
		return err
	}

	for i := range addsStateEvents {
		addsStateEvents[i], err = s.updateStateEvent(addsStateEvents[i])
		if err != nil {
			return err
		}
	}

	if msg.RewritesState {
		if err = s.db.PurgeRoom(ctx, ev.RoomID()); err != nil {
			return fmt.Errorf("s.db.PurgeRoom: %w", err)
		}
	}

	pduPos, err := s.db.WriteEvent(
		ctx,
		&ev,
		addsStateEvents,
		msg.AddsStateEventIDs,
		msg.RemovesStateEventIDs,
		msg.TransactionID,
		false,
	)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
			"add":        msg.AddsStateEventIDs,
			"del":        msg.RemovesStateEventIDs,
		}).Panicf("roomserver output log: write event failure")
		return nil
	}

	if pduPos, err = s.notifyJoinedPeeks(ctx, &ev, pduPos); err != nil {
		logrus.WithError(err).Errorf("Failed to notifyJoinedPeeks for PDU pos %d", pduPos)
		return err
	}

	s.notifier.OnNewEvent(&ev, "", nil, types.NewStreamToken(pduPos, 0, nil))

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
) error {
	pduPos, err := s.db.AddInviteEvent(ctx, msg.Event)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(msg.Event.JSON()),
			"pdupos":     pduPos,
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write invite failure")
		return nil
	}
	s.notifier.OnNewEvent(&msg.Event, "", nil, types.NewStreamToken(pduPos, 0, nil))
	return nil
}

func (s *OutputRoomEventConsumer) onRetireInviteEvent(
	ctx context.Context, msg api.OutputRetireInviteEvent,
) error {
	sp, err := s.db.RetireInviteEvent(ctx, msg.EventID)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   msg.EventID,
			log.ErrorKey: err,
		}).Panicf("roomserver output log: remove invite failure")
		return nil
	}
	// Notify any active sync requests that the invite has been retired.
	// Invites share the same stream counter as PDUs
	s.notifier.OnNewEvent(nil, "", []string{msg.TargetUserID}, types.NewStreamToken(sp, 0, nil))
	return nil
}

func (s *OutputRoomEventConsumer) onNewPeek(
	ctx context.Context, msg api.OutputNewPeek,
) error {
	sp, err := s.db.AddPeek(ctx, msg.RoomID, msg.UserID, msg.DeviceID)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write peek failure")
		return nil
	}
	// tell the notifier about the new peek so it knows to wake up new devices
	s.notifier.OnNewPeek(msg.RoomID, msg.UserID, msg.DeviceID)

	// we need to wake up the users who might need to now be peeking into this room,
	// so we send in a dummy event to trigger a wakeup
	s.notifier.OnNewEvent(nil, msg.RoomID, nil, types.NewStreamToken(sp, 0, nil))
	return nil
}

func (s *OutputRoomEventConsumer) updateStateEvent(event gomatrixserverlib.HeaderedEvent) (gomatrixserverlib.HeaderedEvent, error) {
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

	if prevEvent == nil {
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
