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
	log "github.com/sirupsen/logrus"
)

// OutputSendToDeviceEventConsumer consumes events that originated in the EDU server.
type OutputSendToDeviceEventConsumer struct {
	sendToDeviceConsumer *internal.ContinualConsumer
	db                   storage.Database
	notifier             *sync.Notifier
}

// NewOutputSendToDeviceEventConsumer creates a new OutputSendToDeviceEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputSendToDeviceEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store storage.Database,
) *OutputSendToDeviceEventConsumer {

	consumer := internal.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputSendToDeviceEventTopic),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}

	s := &OutputSendToDeviceEventConsumer{
		sendToDeviceConsumer: &consumer,
		db:                   store,
		notifier:             n,
	}

	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from EDU api
func (s *OutputSendToDeviceEventConsumer) Start() error {
	return s.sendToDeviceConsumer.Start()
}

func (s *OutputSendToDeviceEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output api.OutputSendToDeviceEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("EDU server output log: message parse failure")
		return nil
	}

	log.WithFields(log.Fields{
		"user_id":    output.UserID,
		"device_id":  output.DeviceID,
		"event_type": output.EventType,
	}).Debug("received send-to-device event from EDU server")

	return nil
}
