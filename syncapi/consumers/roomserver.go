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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal/fulltext"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/streams"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	ctx          context.Context
	cfg          *config.SyncAPI
	rsAPI        api.SyncRoomserverAPI
	jetstream    nats.JetStreamContext
	durable      string
	topic        string
	db           storage.Database
	pduStream    streams.StreamProvider
	inviteStream streams.StreamProvider
	notifier     *notifier.Notifier
	fts          fulltext.Indexer
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	notifier *notifier.Notifier,
	pduStream streams.StreamProvider,
	inviteStream streams.StreamProvider,
	rsAPI api.SyncRoomserverAPI,
	fts *fulltext.Search,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:          process.Context(),
		cfg:          cfg,
		jetstream:    js,
		topic:        cfg.Matrix.JetStream.Prefixed(jetstream.OutputRoomEvent),
		durable:      cfg.Matrix.JetStream.Durable("SyncAPIRoomServerConsumer"),
		db:           store,
		notifier:     notifier,
		pduStream:    pduStream,
		inviteStream: inviteStream,
		rsAPI:        rsAPI,
		fts:          fts,
	}
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, 1,
		s.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputRoomEventConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
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
		if event.Type() == spec.MRoomRedaction && event.StateKey() == nil {
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
	case api.OutputTypePurgeRoom:
		err = s.onPurgeRoom(s.ctx, *output.PurgeRoom)
		if err != nil {
			logrus.WithField("room_id", output.PurgeRoom.RoomID).WithError(err).Error("Failed to purge room from sync API")
			return true // non-fatal, as otherwise we end up in a loop of trying to purge the room
		}
	default:
		log.WithField("type", output.Type).Debug(
			"roomserver output log: ignoring unknown output type",
		)
	}
	if err != nil {
		if errors.As(err, new(base64.CorruptInputError)) {
			// no matter how often we retry this event, we will always get this error, discard the event
			return true
		}
		log.WithFields(log.Fields{
			"type": output.Type,
		}).WithError(err).Error("roomserver output log: failed to process event")
		sentry.CaptureException(err)
		return false
	}

	return true
}

func (s *OutputRoomEventConsumer) onRedactEvent(
	ctx context.Context, msg api.OutputRedactedEvent,
) error {
	err := s.db.RedactEvent(ctx, msg.RedactedEventID, msg.RedactedBecause, s.rsAPI)
	if err != nil {
		log.WithError(err).Error("RedactEvent error'd")
		return err
	}

	if err = s.db.RedactRelations(ctx, msg.RedactedBecause.RoomID(), msg.RedactedEventID); err != nil {
		log.WithFields(log.Fields{
			"room_id":           msg.RedactedBecause.RoomID(),
			"event_id":          msg.RedactedBecause.EventID(),
			"redacted_event_id": msg.RedactedEventID,
		}).WithError(err).Warn("Failed to redact relations")
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
	addsStateEvents, missingEventIDs := msg.NeededStateEventIDs()

	// Work out the list of events we need to find out about. Either
	// they will be the event supplied in the request, we will find it
	// in the sync API database or we'll need to ask the roomserver.
	knownEventIDs := make(map[string]bool, len(msg.AddsStateEventIDs))
	for _, eventID := range missingEventIDs {
		knownEventIDs[eventID] = false
	}

	// Look the events up in the database. If we know them, add them into
	// the set of adds state events.
	if len(missingEventIDs) > 0 {
		alreadyKnown, err := s.db.Events(ctx, missingEventIDs)
		if err != nil {
			return fmt.Errorf("s.db.Events: %w", err)
		}
		for _, knownEvent := range alreadyKnown {
			knownEventIDs[knownEvent.EventID()] = true
			addsStateEvents = append(addsStateEvents, knownEvent)
		}
	}

	// Now work out if there are any remaining events we don't know. For
	// these we will need to ask the roomserver for help.
	missingEventIDs = missingEventIDs[:0]
	for eventID, known := range knownEventIDs {
		if !known {
			missingEventIDs = append(missingEventIDs, eventID)
		}
	}

	// Ask the roomserver and add in the rest of the results into the set.
	// Finally, work out if there are any more events missing.
	if len(missingEventIDs) > 0 {
		eventsReq := &api.QueryEventsByIDRequest{
			RoomID:   ev.RoomID(),
			EventIDs: missingEventIDs,
		}
		eventsRes := &api.QueryEventsByIDResponse{}
		if err := s.rsAPI.QueryEventsByID(ctx, eventsReq, eventsRes); err != nil {
			return fmt.Errorf("s.rsAPI.QueryEventsByID: %w", err)
		}
		for _, event := range eventsRes.Events {
			addsStateEvents = append(addsStateEvents, event)
			knownEventIDs[event.EventID()] = true
		}

		// This should never happen because this would imply that the
		// roomserver has sent us adds_state_event_ids for events that it
		// also doesn't know about, but let's just be sure.
		for eventID, found := range knownEventIDs {
			if !found {
				return fmt.Errorf("event %s is missing", eventID)
			}
		}
	}

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
		if err = s.db.PurgeRoomState(ctx, ev.RoomID()); err != nil {
			return fmt.Errorf("s.db.PurgeRoom: %w", err)
		}
	}

	validRoomID, err := spec.NewRoomID(ev.RoomID())
	if err != nil {
		return err
	}

	userID, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, ev.SenderID())
	if err != nil {
		return err
	}

	ev.UserID = *userID

	pduPos, err := s.db.WriteEvent(ctx, ev, addsStateEvents, msg.AddsStateEventIDs, msg.RemovesStateEventIDs, msg.TransactionID, false, msg.HistoryVisibility)
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
	if err = s.writeFTS(ev, pduPos); err != nil {
		log.WithFields(log.Fields{
			"event_id": ev.EventID(),
			"type":     ev.Type(),
		}).WithError(err).Warn("failed to index fulltext element")
	}

	if pduPos, err = s.notifyJoinedPeeks(ctx, ev, pduPos); err != nil {
		log.WithError(err).Errorf("Failed to notifyJoinedPeeks for PDU pos %d", pduPos)
		return err
	}

	if err = s.db.UpdateRelations(ctx, ev); err != nil {
		log.WithFields(log.Fields{
			"event_id": ev.EventID(),
			"type":     ev.Type(),
		}).WithError(err).Warn("Failed to update relations")
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

	validRoomID, err := spec.NewRoomID(ev.RoomID())
	if err != nil {
		return err
	}

	userID, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, ev.SenderID())
	if err != nil {
		return err
	}
	ev.UserID = *userID

	pduPos, err := s.db.WriteEvent(ctx, ev, []*rstypes.HeaderedEvent{}, []string{}, []string{}, nil, ev.StateKey() != nil, msg.HistoryVisibility)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   ev.EventID(),
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write old event failure")
		return nil
	}

	if err = s.writeFTS(ev, pduPos); err != nil {
		log.WithFields(log.Fields{
			"event_id": ev.EventID(),
			"type":     ev.Type(),
		}).WithError(err).Warn("failed to index fulltext element")
	}

	if err = s.db.UpdateRelations(ctx, ev); err != nil {
		log.WithFields(log.Fields{
			"room_id":  ev.RoomID(),
			"event_id": ev.EventID(),
			"type":     ev.Type(),
		}).WithError(err).Warn("Failed to update relations")
		return err
	}

	if pduPos, err = s.notifyJoinedPeeks(ctx, ev, pduPos); err != nil {
		log.WithError(err).Errorf("Failed to notifyJoinedPeeks for PDU pos %d", pduPos)
		return err
	}

	s.pduStream.Advance(pduPos)
	s.notifier.OnNewEvent(ev, ev.RoomID(), nil, types.StreamingToken{PDUPosition: pduPos})

	return nil
}

func (s *OutputRoomEventConsumer) notifyJoinedPeeks(ctx context.Context, ev *rstypes.HeaderedEvent, sp types.StreamPosition) (types.StreamPosition, error) {
	if ev.Type() != spec.MRoomMember {
		return sp, nil
	}
	membership, err := ev.Membership()
	if err != nil {
		return sp, fmt.Errorf("ev.Membership: %w", err)
	}
	// TODO: check that it's a join and not a profile change (means unmarshalling prev_content)
	if membership == spec.Join {
		// check it's a local join
		if ev.StateKey() == nil {
			return sp, fmt.Errorf("unexpected nil state_key")
		}

		validRoomID, err := spec.NewRoomID(ev.RoomID())
		if err != nil {
			return sp, err
		}
		userID, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, spec.SenderID(*ev.StateKey()))
		if err != nil || userID == nil {
			return sp, fmt.Errorf("failed getting userID for sender: %w", err)
		}
		if !s.cfg.Matrix.IsLocalServerName(userID.Domain()) {
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
		return
	}

	validRoomID, err := spec.NewRoomID(msg.Event.RoomID())
	if err != nil {
		return
	}
	userID, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, spec.SenderID(*msg.Event.StateKey()))
	if err != nil || userID == nil {
		return
	}
	if !s.cfg.Matrix.IsLocalServerName(userID.Domain()) {
		return
	}

	msg.Event.UserID = *userID

	pduPos, err := s.db.AddInviteEvent(ctx, msg.Event)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   msg.Event.EventID(),
			"event":      string(msg.Event.JSON()),
			"pdupos":     pduPos,
			log.ErrorKey: err,
		}).Errorf("roomserver output log: write invite failure")
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
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   msg.EventID,
			log.ErrorKey: err,
		}).Errorf("roomserver output log: remove invite failure")
		return
	}

	// Only notify clients about retired invite events, if the user didn't accept the invite.
	// The PDU stream will also receive an event about accepting the invitation, so there should
	// be a "smooth" transition from invite -> join, and not invite -> leave -> join
	if msg.Membership == spec.Join {
		return
	}

	// Notify any active sync requests that the invite has been retired.
	s.inviteStream.Advance(pduPos)
	validRoomID, err := spec.NewRoomID(msg.RoomID)
	if err != nil {
		log.WithFields(log.Fields{
			"event_id":   msg.EventID,
			"room_id":    msg.RoomID,
			log.ErrorKey: err,
		}).Errorf("roomID is invalid")
		return
	}
	userID, err := s.rsAPI.QueryUserIDForSender(ctx, *validRoomID, msg.TargetSenderID)
	if err != nil || userID == nil {
		log.WithFields(log.Fields{
			"event_id":   msg.EventID,
			"sender_id":  msg.TargetSenderID,
			log.ErrorKey: err,
		}).Errorf("failed to find userID for sender")
		return
	}
	s.notifier.OnNewInvite(types.StreamingToken{InvitePosition: pduPos}, userID.String())
}

func (s *OutputRoomEventConsumer) onNewPeek(
	ctx context.Context, msg api.OutputNewPeek,
) {
	sp, err := s.db.AddPeek(ctx, msg.RoomID, msg.UserID, msg.DeviceID)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			log.ErrorKey: err,
		}).Errorf("roomserver output log: write peek failure")
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
		}).Errorf("roomserver output log: write peek failure")
		return
	}

	// tell the notifier about the new peek so it knows to wake up new devices
	// TODO: This only works because the peeks table is reusing the same
	// index as PDUs, but we should fix this
	s.pduStream.Advance(sp)
	s.notifier.OnRetirePeek(msg.RoomID, msg.UserID, msg.DeviceID, types.StreamingToken{PDUPosition: sp})
}

func (s *OutputRoomEventConsumer) onPurgeRoom(
	ctx context.Context, req api.OutputPurgeRoom,
) error {
	logrus.WithField("room_id", req.RoomID).Warn("Purging room from sync API")

	if err := s.db.PurgeRoom(ctx, req.RoomID); err != nil {
		logrus.WithField("room_id", req.RoomID).WithError(err).Error("Failed to purge room from sync API")
		return err
	} else {
		logrus.WithField("room_id", req.RoomID).Warn("Room purged from sync API")
		return nil
	}
}

func (s *OutputRoomEventConsumer) updateStateEvent(event *rstypes.HeaderedEvent) (*rstypes.HeaderedEvent, error) {
	event.StateKeyResolved = event.StateKey()
	if event.StateKey() == nil {
		return event, nil
	}
	stateKey := *event.StateKey()

	snapshot, err := s.db.NewDatabaseSnapshot(s.ctx)
	if err != nil {
		return nil, err
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)

	validRoomID, err := spec.NewRoomID(event.RoomID())
	if err != nil {
		return event, err
	}

	sKeyUser := ""
	if stateKey != "" {
		var sku *spec.UserID
		sku, err = s.rsAPI.QueryUserIDForSender(s.ctx, *validRoomID, spec.SenderID(stateKey))
		if err == nil && sku != nil {
			sKeyUser = sku.String()
			event.StateKeyResolved = &sKeyUser
		}
	}

	prevEvent, err := snapshot.GetStateEvent(
		s.ctx, event.RoomID(), event.Type(), sKeyUser,
	)
	if err != nil {
		return event, err
	}

	userID, err := s.rsAPI.QueryUserIDForSender(s.ctx, *validRoomID, event.SenderID())
	if err != nil {
		return event, err
	}

	event.UserID = *userID

	if prevEvent == nil || prevEvent.EventID() == event.EventID() {
		return event, nil
	}

	prevEventSender := string(prevEvent.SenderID())
	prevUser, err := s.rsAPI.QueryUserIDForSender(s.ctx, *validRoomID, prevEvent.SenderID())
	if err == nil && prevUser != nil {
		prevEventSender = prevUser.String()
	}

	prev := types.PrevEventRef{
		PrevContent:   prevEvent.Content(),
		ReplacesState: prevEvent.EventID(),
		PrevSenderID:  prevEventSender,
	}

	event.PDU, err = event.SetUnsigned(prev)
	succeeded = true
	return event, err
}

func (s *OutputRoomEventConsumer) writeFTS(ev *rstypes.HeaderedEvent, pduPosition types.StreamPosition) error {
	if !s.cfg.Fulltext.Enabled {
		return nil
	}
	e := fulltext.IndexElement{
		EventID:        ev.EventID(),
		RoomID:         ev.RoomID(),
		StreamPosition: int64(pduPosition),
	}
	e.SetContentType(ev.Type())

	switch ev.Type() {
	case "m.room.message":
		e.Content = gjson.GetBytes(ev.Content(), "body").String()
	case spec.MRoomName:
		e.Content = gjson.GetBytes(ev.Content(), "name").String()
	case spec.MRoomTopic:
		e.Content = gjson.GetBytes(ev.Content(), "topic").String()
	case spec.MRoomRedaction:
		log.Tracef("Redacting event: %s", ev.Redacts())
		if err := s.fts.Delete(ev.Redacts()); err != nil {
			return fmt.Errorf("failed to delete entry from fulltext index: %w", err)
		}
		return nil
	default:
		return nil
	}
	if e.Content != "" {
		log.Tracef("Indexing element: %+v", e)
		if err := s.fts.Index(e); err != nil {
			return err
		}
	}
	return nil
}
