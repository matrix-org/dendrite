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
	"encoding/json"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// OutputClientData consumes events that originated in the client API server.
type OutputClientData struct {
	clientAPIConsumer *common.ContinualConsumer
	db                *storage.SyncServerDatabase
	notifier          *sync.Notifier
}

// NewOutputClientData creates a new OutputClientData consumer. Call Start() to begin consuming from room servers.
func NewOutputClientData(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store *storage.SyncServerDatabase,
) *OutputClientData {

	consumer := common.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputClientData),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputClientData{
		clientAPIConsumer: &consumer,
		db:                store,
		notifier:          n,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from room servers
func (s *OutputClientData) Start() error {
	return s.clientAPIConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the client API server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputClientData) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output common.AccountData
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("client API server output log: message parse failure")
		return nil
	}

	log.WithFields(log.Fields{
		"type":    output.Type,
		"room_id": output.RoomID,
	}).Info("received data from client API server")

	syncStreamPos, err := s.db.UpsertAccountData(string(msg.Key), output.RoomID, output.Type)
	if err != nil {
		log.WithFields(log.Fields{
			"type":       output.Type,
			"room_id":    output.RoomID,
			log.ErrorKey: err,
		}).Panicf("could not save account data")
	}

	s.notifier.OnNewEvent(nil, string(msg.Key), syncStreamPos)

	return nil
}
