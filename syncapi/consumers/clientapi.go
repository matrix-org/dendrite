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

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputClientDataConsumer consumes events that originated in the client API server.
type OutputClientDataConsumer struct {
	ctx       context.Context
	jetstream nats.JetStreamContext
	topic     string
	db        storage.Database
	stream    types.StreamProvider
	notifier  *notifier.Notifier
}

// NewOutputClientDataConsumer creates a new OutputClientData consumer. Call Start() to begin consuming from room servers.
func NewOutputClientDataConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
) *OutputClientDataConsumer {
	return &OutputClientDataConsumer{
		ctx:       process.Context(),
		jetstream: js,
		topic:     cfg.Matrix.JetStream.TopicFor(jetstream.OutputClientData),
		db:        store,
		notifier:  notifier,
		stream:    stream,
	}
}

// Start consuming from room servers
func (s *OutputClientDataConsumer) Start() error {
	_, err := s.jetstream.Subscribe(s.topic, s.onMessage)
	return err
}

// onMessage is called when the sync server receives a new event from the client API server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputClientDataConsumer) onMessage(msg *nats.Msg) {
	jetstream.WithJetStreamMessage(msg, func(msg *nats.Msg) bool {
		// Parse out the event JSON
		userID := msg.Header.Get(jetstream.UserID)
		var output eventutil.AccountData
		if err := json.Unmarshal(msg.Data, &output); err != nil {
			// If the message was invalid, log it and move on to the next message in the stream
			log.WithError(err).Errorf("client API server output log: message parse failure")
			sentry.CaptureException(err)
			return true
		}

		log.WithFields(log.Fields{
			"type":    output.Type,
			"room_id": output.RoomID,
		}).Info("received data from client API server")

		streamPos, err := s.db.UpsertAccountData(
			s.ctx, userID, output.RoomID, output.Type,
		)
		if err != nil {
			sentry.CaptureException(err)
			log.WithFields(log.Fields{
				"type":       output.Type,
				"room_id":    output.RoomID,
				log.ErrorKey: err,
			}).Panicf("could not save account data")
		}

		s.stream.Advance(streamPos)
		s.notifier.OnNewAccountData(userID, types.StreamingToken{AccountDataPosition: streamPos})

		return true
	})
}
