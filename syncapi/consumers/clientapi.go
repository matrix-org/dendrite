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
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	log "github.com/sirupsen/logrus"
)

// OutputClientDataConsumer consumes events that originated in the client API server.
type OutputClientDataConsumer struct {
	clientAPIConsumer *internal.ContinualConsumer
	db                storage.Database
	notifier          *sync.Notifier
}

// NewOutputClientDataConsumer creates a new OutputClientData consumer. Call Start() to begin consuming from room servers.
func NewOutputClientDataConsumer(
	cfg *config.SyncAPI,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store storage.Database,
) *OutputClientDataConsumer {

	consumer := internal.ContinualConsumer{
		ComponentName:  "syncapi/clientapi",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputClientData)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputClientDataConsumer{
		clientAPIConsumer: &consumer,
		db:                store,
		notifier:          n,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from room servers
func (s *OutputClientDataConsumer) Start() error {
	return s.clientAPIConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the client API server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputClientDataConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output eventutil.AccountData
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("client API server output log: message parse failure")
		return nil
	}

	log.WithFields(log.Fields{
		"type":    output.Type,
		"room_id": output.RoomID,
	}).Info("received data from client API server")

	pduPos, err := s.db.UpsertAccountData(
		context.TODO(), string(msg.Key), output.RoomID, output.Type,
	)
	if err != nil {
		log.WithFields(log.Fields{
			"type":       output.Type,
			"room_id":    output.RoomID,
			log.ErrorKey: err,
		}).Panicf("could not save account data")
	}

	s.notifier.OnNewEvent(nil, "", []string{string(msg.Key)}, types.NewStreamToken(pduPos, 0, nil))

	return nil
}
