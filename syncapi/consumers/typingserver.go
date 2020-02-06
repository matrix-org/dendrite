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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/typingserver/api"
	log "github.com/sirupsen/logrus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputTypingEventConsumer consumes events that originated in the typing server.
type OutputTypingEventConsumer struct {
	typingConsumer *common.ContinualConsumer
	db             storage.Database
	notifier       *sync.Notifier
}

// NewOutputTypingEventConsumer creates a new OutputTypingEventConsumer.
// Call Start() to begin consuming from the typing server.
func NewOutputTypingEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store storage.Database,
) *OutputTypingEventConsumer {

	consumer := common.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputTypingEvent),
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

// Start consuming from typing api
func (s *OutputTypingEventConsumer) Start() error {
	s.db.SetTypingTimeoutCallback(func(userID, roomID string, latestSyncPosition int64) {
		s.notifier.OnNewEvent(
			nil, roomID, nil,
			types.PaginationToken{
				EDUTypingPosition: types.StreamPosition(latestSyncPosition),
			},
		)
	})

	return s.typingConsumer.Start()
}

func (s *OutputTypingEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output api.OutputTypingEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("typing server output log: message parse failure")
		return nil
	}

	log.WithFields(log.Fields{
		"room_id": output.Event.RoomID,
		"user_id": output.Event.UserID,
		"typing":  output.Event.Typing,
	}).Debug("received data from typing server")

	var typingPos types.StreamPosition
	typingEvent := output.Event
	if typingEvent.Typing {
		typingPos = s.db.AddTypingUser(typingEvent.UserID, typingEvent.RoomID, output.ExpireTime)
	} else {
		typingPos = s.db.RemoveTypingUser(typingEvent.UserID, typingEvent.RoomID)
	}

	s.notifier.OnNewEvent(nil, output.Event.RoomID, nil, types.PaginationToken{EDUTypingPosition: typingPos})
	return nil
}
