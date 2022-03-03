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

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
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
	stream    types.StreamProvider
}

// NewOutputNotificationDataConsumer creates a new consumer. Call
// Start() to begin consuming.
func NewOutputNotificationDataConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
) *OutputNotificationDataConsumer {
	s := &OutputNotificationDataConsumer{
		ctx:       process.Context(),
		jetstream: js,
		durable:   cfg.Matrix.JetStream.Durable("SyncAPINotificationDataConsumer"),
		topic:     cfg.Matrix.JetStream.TopicFor(jetstream.OutputNotificationData),
		db:        store,
		notifier:  notifier,
		stream:    stream,
	}
	return s
}

// Start starts consumption.
func (s *OutputNotificationDataConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called when the Sync server receives a new event from
// the push server. It is not safe for this function to be called from
// multiple goroutines, or else the sync stream position may race and
// be incorrectly calculated.
func (s *OutputNotificationDataConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
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
