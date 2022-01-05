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
	"context"
	"encoding/json"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputTypingEventConsumer consumes events that originated in the EDU server.
type OutputTypingEventConsumer struct {
	ctx       context.Context
	jetstream nats.JetStreamContext
	topic     string
	eduCache  *cache.EDUCache
	stream    types.StreamProvider
	notifier  *notifier.Notifier
}

// NewOutputTypingEventConsumer creates a new OutputTypingEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputTypingEventConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	eduCache *cache.EDUCache,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
) *OutputTypingEventConsumer {
	return &OutputTypingEventConsumer{
		ctx:       process.Context(),
		jetstream: js,
		topic:     cfg.Matrix.JetStream.TopicFor(jetstream.OutputTypingEvent),
		eduCache:  eduCache,
		notifier:  notifier,
		stream:    stream,
	}
}

// Start consuming from EDU api
func (s *OutputTypingEventConsumer) Start() error {
	_, err := s.jetstream.Subscribe(s.topic, s.onMessage)
	return err
}

func (s *OutputTypingEventConsumer) onMessage(msg *nats.Msg) {
	jetstream.WithJetStreamMessage(msg, func(msg *nats.Msg) bool {
		var output api.OutputTypingEvent
		if err := json.Unmarshal(msg.Data, &output); err != nil {
			// If the message was invalid, log it and move on to the next message in the stream
			log.WithError(err).Errorf("EDU server output log: message parse failure")
			sentry.CaptureException(err)
			return true
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

		return true
	})
}
