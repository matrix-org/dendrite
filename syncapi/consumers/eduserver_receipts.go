// Copyright 2020 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/syncapi/types"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	log "github.com/sirupsen/logrus"
)

// OutputReceiptEventConsumer consumes events that originated in the EDU server.
type OutputReceiptEventConsumer struct {
	receiptConsumer *internal.ContinualConsumer
	db              storage.Database
	notifier        *sync.Notifier
}

// NewOutputReceiptEventConsumer creates a new OutputReceiptEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputReceiptEventConsumer(
	cfg *config.SyncAPI,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store storage.Database,
) *OutputReceiptEventConsumer {

	consumer := internal.ContinualConsumer{
		ComponentName:  "syncapi/eduserver/receipt",
		Topic:          cfg.Matrix.Kafka.TopicFor(config.TopicOutputReceiptEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}

	s := &OutputReceiptEventConsumer{
		receiptConsumer: &consumer,
		db:              store,
		notifier:        n,
	}

	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from EDU api
func (s *OutputReceiptEventConsumer) Start() error {
	return s.receiptConsumer.Start()
}

func (s *OutputReceiptEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output api.OutputReceiptEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("EDU server output log: message parse failure")
		return nil
	}

	streamPos, err := s.db.StoreReceipt(
		context.TODO(),
		output.RoomID,
		output.Type,
		output.UserID,
		output.EventID,
		output.Timestamp,
	)
	if err != nil {
		return err
	}
	// update stream position
	s.notifier.OnNewReceipt(types.NewStreamToken(0, streamPos, nil))

	return nil
}
