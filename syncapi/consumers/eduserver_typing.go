// Copyright 2019 Alex Chen
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
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	log "github.com/sirupsen/logrus"
)

// OutputTypingEventConsumer consumes events that originated in the EDU server.
type OutputTypingEventConsumer struct {
	typingConsumer *internal.ContinualConsumer
	db             storage.Database
	notifier       *sync.Notifier
}

// NewOutputTypingEventConsumer creates a new OutputTypingEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputTypingEventConsumer(
	cfg *config.SyncAPI,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store storage.Database,
) *OutputTypingEventConsumer {

	consumer := internal.ContinualConsumer{
		ComponentName:  "syncapi/eduserver/typing",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputTypingEvent)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}

	s := &OutputTypingEventConsumer{
		typingConsumer: &consumer,
		db:             store,
		notifier:       n,
	}

	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from EDU api
func (s *OutputTypingEventConsumer) Start() error {
	s.db.SetTypingTimeoutCallback(func(userID, roomID string, latestSyncPosition int64) {
		s.notifier.OnNewEvent(
			nil, roomID, nil,
			types.NewStreamToken(0, types.StreamPosition(latestSyncPosition), nil),
		)
	})

	return s.typingConsumer.Start()
}

func (s *OutputTypingEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output api.OutputTypingEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("EDU server output log: message parse failure")
		return nil
	}

	log.WithFields(log.Fields{
		"room_id": output.Event.RoomID,
		"user_id": output.Event.UserID,
		"typing":  output.Event.Typing,
	}).Debug("received data from EDU server")

	var typingPos types.StreamPosition
	typingEvent := output.Event
	if typingEvent.Typing {
		typingPos = s.db.AddTypingUser(typingEvent.UserID, typingEvent.RoomID, output.ExpireTime)
	} else {
		typingPos = s.db.RemoveTypingUser(typingEvent.UserID, typingEvent.RoomID)
	}

	s.notifier.OnNewEvent(nil, output.Event.RoomID, nil, types.NewStreamToken(0, typingPos, nil))
	return nil
}
