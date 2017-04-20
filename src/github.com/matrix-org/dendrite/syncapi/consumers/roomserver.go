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
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// Server contains all the logic for running a sync server
type Server struct {
	roomServerConsumer *common.ContinualConsumer
	db                 *storage.SyncServerDatabase
	rp                 *sync.RequestPool
}

// NewServer creates a new sync server. Call Start() to begin consuming from room servers.
func NewServer(cfg *config.Sync, rp *sync.RequestPool, store *storage.SyncServerDatabase) (*Server, error) {
	kafkaConsumer, err := sarama.NewConsumer(cfg.KafkaConsumerURIs, nil)
	if err != nil {
		return nil, err
	}

	consumer := common.ContinualConsumer{
		Topic:          cfg.RoomserverOutputTopic,
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &Server{
		roomServerConsumer: &consumer,
		db:                 store,
		rp:                 rp,
	}
	consumer.ProcessMessage = s.onMessage

	return s, nil
}

// Start consuming from room servers
func (s *Server) Start() error {
	return s.roomServerConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *Server) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output api.OutputRoomEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	ev, err := gomatrixserverlib.NewEventFromTrustedJSON(output.Event, false)
	if err != nil {
		log.WithError(err).Errorf("roomserver output log: event parse failure")
		return nil
	}
	log.WithFields(log.Fields{
		"event_id": ev.EventID(),
		"room_id":  ev.RoomID(),
	}).Info("received event from roomserver")

	syncStreamPos, err := s.db.WriteEvent(&ev, output.AddsStateEventIDs, output.RemovesStateEventIDs)

	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
			"add":        output.AddsStateEventIDs,
			"del":        output.RemovesStateEventIDs,
		}).Panicf("roomserver output log: write event failure")
		return nil
	}
	s.rp.OnNewEvent(&ev, types.StreamPosition(syncStreamPos))

	return nil
}
