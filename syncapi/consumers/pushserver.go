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

	"github.com/Shopify/sarama"
	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	log "github.com/sirupsen/logrus"
)

// OutputNotificationDataConsumer consumes events that originated in
// the Push server.
type OutputNotificationDataConsumer struct {
	consumer *internal.ContinualConsumer
	db       storage.Database
	stream   types.StreamProvider
	notifier *notifier.Notifier
}

// NewOutputNotificationDataConsumer creates a new consumer. Call
// Start() to begin consuming.
func NewOutputNotificationDataConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
) *OutputNotificationDataConsumer {
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "syncapi/pushserver",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputNotificationData)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputNotificationDataConsumer{
		consumer: &consumer,
		db:       store,
		notifier: notifier,
		stream:   stream,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start starts consumption.
func (s *OutputNotificationDataConsumer) Start() error {
	return s.consumer.Start()
}

// onMessage is called when the Sync server receives a new event from
// the push server. It is not safe for this function to be called from
// multiple goroutines, or else the sync stream position may race and
// be incorrectly calculated.
func (s *OutputNotificationDataConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	ctx := context.Background()

	userID := string(msg.Key)

	// Parse out the event JSON
	var data eventutil.NotificationData
	if err := json.Unmarshal(msg.Value, &data); err != nil {
		sentry.CaptureException(err)
		log.WithField("user_id", userID).WithError(err).Error("push server consumer: message parse failure")
		return nil
	}

	streamPos, err := s.db.UpsertRoomUnreadNotificationCounts(ctx, userID, data.RoomID, data.UnreadNotificationCount, data.UnreadHighlightCount)
	if err != nil {
		sentry.CaptureException(err)
		log.WithFields(log.Fields{
			"user_id": userID,
			"room_id": data.RoomID,
		}).WithError(err).Panic("Could not save notification counts")
	}

	s.stream.Advance(streamPos)
	s.notifier.OnNewNotificationData(userID, types.StreamingToken{NotificationDataPosition: streamPos})

	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   data.RoomID,
		"streamPos": streamPos,
	}).Info("Received data from Push server")

	return nil
}
