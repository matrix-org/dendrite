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

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputReceiptEventConsumer consumes events that originated in the EDU server.
type OutputReceiptEventConsumer struct {
	ctx       context.Context
	jetstream nats.JetStreamContext
	topic     string
	db        storage.Database
	stream    types.StreamProvider
	notifier  *notifier.Notifier
}

// NewOutputReceiptEventConsumer creates a new OutputReceiptEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputReceiptEventConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
) *OutputReceiptEventConsumer {
	return &OutputReceiptEventConsumer{
		ctx:       process.Context(),
		jetstream: js,
		topic:     cfg.Matrix.JetStream.TopicFor(jetstream.OutputReceiptEvent),
		db:        store,
		notifier:  notifier,
		stream:    stream,
	}
}

// Start consuming from EDU api
func (s *OutputReceiptEventConsumer) Start() error {
	_, err := s.jetstream.Subscribe(s.topic, s.onMessage)
	return err
}

func (s *OutputReceiptEventConsumer) onMessage(msg *nats.Msg) {
	jetstream.WithJetStreamMessage(msg, func(msg *nats.Msg) bool {
		var output api.OutputReceiptEvent
		if err := json.Unmarshal(msg.Data, &output); err != nil {
			// If the message was invalid, log it and move on to the next message in the stream
			log.WithError(err).Errorf("EDU server output log: message parse failure")
			sentry.CaptureException(err)
			return true
		}

		streamPos, err := s.db.StoreReceipt(
			s.ctx,
			output.RoomID,
			output.Type,
			output.UserID,
			output.EventID,
			output.Timestamp,
		)
		if err != nil {
			sentry.CaptureException(err)
			return true
		}

		s.stream.Advance(streamPos)
		s.notifier.OnNewReceipt(output.RoomID, types.StreamingToken{ReceiptPosition: streamPos})

		return true
	})
}
