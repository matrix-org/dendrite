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
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	log "github.com/sirupsen/logrus"
)

// OutputTypingEventConsumer consumes events that originated in the EDU server.
type OutputTypingEventConsumer struct {
	typingConsumer *internal.ContinualConsumer
	eduCache       *cache.EDUCache
	stream         types.StreamProvider
	notifier       *notifier.Notifier
}

// NewOutputTypingEventConsumer creates a new OutputTypingEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputTypingEventConsumer(
	cfg *config.SyncAPI,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
	eduCache *cache.EDUCache,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
) *OutputTypingEventConsumer {

	consumer := internal.ContinualConsumer{
		ComponentName:  "syncapi/eduserver/typing",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputTypingEvent)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}

	s := &OutputTypingEventConsumer{
		typingConsumer: &consumer,
		eduCache:       eduCache,
		notifier:       notifier,
		stream:         stream,
	}

	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from EDU api
func (s *OutputTypingEventConsumer) Start() error {
	s.eduCache.SetTimeoutCallback(func(userID, roomID string, latestSyncPosition int64) {
		pos := types.StreamPosition(latestSyncPosition)
		s.notifier.OnNewTyping(roomID, types.StreamingToken{TypingPosition: pos})
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
		typingPos = types.StreamPosition(
			s.eduCache.AddTypingUser(typingEvent.UserID, typingEvent.RoomID, output.ExpireTime),
		)
	} else {
		typingPos = types.StreamPosition(
			s.eduCache.RemoveUser(typingEvent.UserID, typingEvent.RoomID),
		)
	}

	s.stream.Advance(typingPos)
	s.notifier.OnNewTyping(output.Event.RoomID, types.StreamingToken{TypingPosition: typingPos})

	return nil
}
