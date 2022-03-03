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
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/producers"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

// OutputReceiptEventConsumer consumes events that originated in the EDU server.
type OutputReceiptEventConsumer struct {
	ctx        context.Context
	jetstream  nats.JetStreamContext
	durable    string
	topic      string
	db         storage.Database
	stream     types.StreamProvider
	notifier   *notifier.Notifier
	serverName gomatrixserverlib.ServerName
	producer   *producers.UserAPIReadProducer
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
	producer *producers.UserAPIReadProducer,
) *OutputReceiptEventConsumer {
	return &OutputReceiptEventConsumer{
		ctx:        process.Context(),
		jetstream:  js,
		topic:      cfg.Matrix.JetStream.TopicFor(jetstream.OutputReceiptEvent),
		durable:    cfg.Matrix.JetStream.Durable("SyncAPIEDUServerReceiptConsumer"),
		db:         store,
		notifier:   notifier,
		stream:     stream,
		serverName: cfg.Matrix.ServerName,
		producer:   producer,
	}
}

// Start consuming from EDU api
func (s *OutputReceiptEventConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

func (s *OutputReceiptEventConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
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

	if err = s.sendReadUpdate(ctx, output); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"user_id": output.UserID,
			"room_id": output.RoomID,
		}).Errorf("Failed to generate read update")
		sentry.CaptureException(err)
		return false
	}

	s.stream.Advance(streamPos)
	s.notifier.OnNewReceipt(output.RoomID, types.StreamingToken{ReceiptPosition: streamPos})

	return true
}

func (s *OutputReceiptEventConsumer) sendReadUpdate(ctx context.Context, output api.OutputReceiptEvent) error {
	if output.Type != "m.read" {
		return nil
	}
	_, serverName, err := gomatrixserverlib.SplitID('@', output.UserID)
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
	}
	if serverName != s.serverName {
		return nil
	}
	var readPos types.StreamPosition
	if output.EventID != "" {
		if _, readPos, err = s.db.PositionInTopology(ctx, output.EventID); err != nil {
			return fmt.Errorf("s.db.PositionInTopology (Read): %w", err)
		}
	}
	if readPos > 0 {
		if err := s.producer.SendReadUpdate(output.UserID, output.RoomID, readPos, 0); err != nil {
			return fmt.Errorf("s.producer.SendReadUpdate: %w", err)
		}
	}
	return nil
}
