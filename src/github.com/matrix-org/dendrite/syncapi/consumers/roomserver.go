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

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputRoomEvent consumes events that originated in the room server.
type OutputRoomEvent struct {
	roomServerConsumer *common.ContinualConsumer
	db                 *storage.SyncServerDatabase
	notifier           *sync.Notifier
	query              api.RoomserverQueryAPI
}

// NewOutputRoomEvent creates a new OutputRoomEvent consumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEvent(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store *storage.SyncServerDatabase,
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
		notifier:           n,
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

	switch output.Type {
	case api.OutputTypeNewRoomEvent:
		return s.onNewRoomEvent(context.TODO(), *output.NewRoomEvent)
	case api.OutputTypeNewInviteEvent:
		return s.onNewInviteEvent(context.TODO(), *output.NewInviteEvent)
	case api.OutputTypeRetireInviteEvent:
		return s.onRetireInviteEvent(context.TODO(), *output.RetireInviteEvent)
	default:
		log.WithField("type", output.Type).Debug(
			"roomserver output log: ignoring unknown output type",
		)
		return nil
	}
}

func (s *OutputRoomEvent) onNewRoomEvent(
	ctx context.Context, msg api.OutputNewRoomEvent,
) error {
	ev := msg.Event
	log.WithFields(log.Fields{
		"event_id": ev.EventID(),
		"room_id":  ev.RoomID(),
	}).Info("received event from roomserver")

	addsStateEvents, err := s.lookupStateEvents(msg.AddsStateEventIDs, ev)
	if err != nil {
		log.WithFields(log.Fields{
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
			"add":        msg.AddsStateEventIDs,
			"del":        msg.RemovesStateEventIDs,
		}).Panicf("roomserver output log: state event lookup failure")
	}

	ev, err = s.updateStateEvent(ev)
	if err != nil {
		return err
	}

	for i := range addsStateEvents {
		addsStateEvents[i], err = s.updateStateEvent(addsStateEvents[i])
		if err != nil {
			return err
		}
	}

	syncStreamPos, err := s.db.WriteEvent(
		ctx,
		&ev,
		addsStateEvents,
		msg.AddsStateEventIDs,
		msg.RemovesStateEventIDs,
	)
	if err != nil {
		return err
	}

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
	s.notifier.OnNewEvent(&ev, "", types.StreamPosition(syncStreamPos))

	return nil
}

func (s *OutputRoomEvent) onNewInviteEvent(
	ctx context.Context, msg api.OutputNewInviteEvent,
) error {
	syncStreamPos, err := s.db.AddInviteEvent(ctx, msg.Event)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(msg.Event.JSON()),
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write invite failure")
		return nil
	}
	s.notifier.OnNewEvent(&msg.Event, "", syncStreamPos)
	return nil
}

func (s *OutputRoomEvent) onRetireInviteEvent(
	ctx context.Context, msg api.OutputRetireInviteEvent,
) error {
	err := s.db.RetireInviteEvent(ctx, msg.EventID)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   msg.EventID,
			log.ErrorKey: err,
		}).Panicf("roomserver output log: remove invite failure")
		return nil
	}
	// TODO: Notify any active sync requests that the invite has been retired.
	// s.notifier.OnNewEvent(nil, msg.TargetUserID, syncStreamPos)
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

	// Check if this is re-adding a state events that we previously processed
	// If we have previously received a state event it may still be in
	// our event database.
	result, err := s.db.Events(context.TODO(), addsStateEventIDs)
	if err != nil {
		return nil, err
	}
	missing := missingEventsFrom(result, addsStateEventIDs)

	// Check if event itself is being added.
	for _, eventID := range missing {
		if eventID == event.EventID() {
			result = append(result, event)
			break
		}
	}
	missing = missingEventsFrom(result, addsStateEventIDs)

	if len(missing) == 0 {
		return result, nil
	}

	// At this point the missing events are neither the event itself nor are
	// they present in our local database. Our only option is to fetch them
	// from the roomserver using the query API.
	eventReq := api.QueryEventsByIDRequest{EventIDs: missing}
	var eventResp api.QueryEventsByIDResponse
	if err := s.query.QueryEventsByID(context.TODO(), &eventReq, &eventResp); err != nil {
		return nil, err
	}

	result = append(result, eventResp.Events...)
	missing = missingEventsFrom(result, addsStateEventIDs)

	if len(missing) != 0 {
		return nil, fmt.Errorf(
			"missing %d state events IDs at event %q", len(missing), event.EventID(),
		)
	}

	return result, nil
}

func (s *OutputRoomEvent) updateStateEvent(event gomatrixserverlib.Event) (gomatrixserverlib.Event, error) {
	var stateKey string
	if event.StateKey() == nil {
		stateKey = ""
	} else {
		stateKey = *event.StateKey()
	}

	prevEvent, err := s.db.GetStateEvent(
		context.TODO(), event.Type(), event.RoomID(), stateKey,
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

	return event.SetUnsigned(prev)
}

func missingEventsFrom(events []gomatrixserverlib.Event, required []string) []string {
	have := map[string]bool{}
	for _, event := range events {
		have[event.EventID()] = true
	}
	var missing []string
	for _, eventID := range required {
		if !have[eventID] {
			missing = append(missing, eventID)
		}
	}
	return missing
}
