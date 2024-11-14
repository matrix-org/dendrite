// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"context"
	"encoding/json"

	"github.com/getsentry/sentry-go"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/syncapi/notifier"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/streams"
	"github.com/element-hq/dendrite/syncapi/types"
)

// OutputNotificationDataConsumer consumes events that originated in
// the Push server.
type OutputNotificationDataConsumer struct {
	ctx       context.Context
	jetstream nats.JetStreamContext
	durable   string
	topic     string
	db        storage.Database
	notifier  *notifier.Notifier
	stream    streams.StreamProvider
}

// NewOutputNotificationDataConsumer creates a new consumer. Call
// Start() to begin consuming.
func NewOutputNotificationDataConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	notifier *notifier.Notifier,
	stream streams.StreamProvider,
) *OutputNotificationDataConsumer {
	s := &OutputNotificationDataConsumer{
		ctx:       process.Context(),
		jetstream: js,
		durable:   cfg.Matrix.JetStream.Durable("SyncAPINotificationDataConsumer"),
		topic:     cfg.Matrix.JetStream.Prefixed(jetstream.OutputNotificationData),
		db:        store,
		notifier:  notifier,
		stream:    stream,
	}
	return s
}

// Start starts consumption.
func (s *OutputNotificationDataConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, 1,
		s.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called when the Sync server receives a new event from
// the push server. It is not safe for this function to be called from
// multiple goroutines, or else the sync stream position may race and
// be incorrectly calculated.
func (s *OutputNotificationDataConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	userID := string(msg.Header.Get(jetstream.UserID))

	// Parse out the event JSON
	var data eventutil.NotificationData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		sentry.CaptureException(err)
		log.WithField("user_id", userID).WithError(err).Error("user API consumer: message parse failure")
		return true
	}

	streamPos, err := s.db.UpsertRoomUnreadNotificationCounts(ctx, userID, data.RoomID, data.UnreadNotificationCount, data.UnreadHighlightCount)
	if err != nil {
		sentry.CaptureException(err)
		log.WithFields(log.Fields{
			"user_id": userID,
			"room_id": data.RoomID,
		}).WithError(err).Error("Could not save notification counts")
		return false
	}

	s.stream.Advance(streamPos)
	s.notifier.OnNewNotificationData(userID, types.StreamingToken{NotificationDataPosition: streamPos})

	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   data.RoomID,
		"streamPos": streamPos,
	}).Trace("Received notification data from user API")

	return true
}
