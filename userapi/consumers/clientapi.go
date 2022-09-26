// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/userapi/storage"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/util"
)

// OutputReceiptEventConsumer consumes events that originated in the EDU server.
type OutputClientDataConsumer struct {
	ctx          context.Context
	jetstream    nats.JetStreamContext
	durable      string
	topic        string
	db           storage.Database
	serverName   gomatrixserverlib.ServerName
	syncProducer *producers.SyncAPI
	pgClient     pushgateway.Client
}

// NewOutputReceiptEventConsumer creates a new OutputReceiptEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputClientDataConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.Database,
	syncProducer *producers.SyncAPI,
	pgClient pushgateway.Client,
) *OutputReceiptEventConsumer {
	return &OutputReceiptEventConsumer{
		ctx:          process.Context(),
		jetstream:    js,
		topic:        cfg.Matrix.JetStream.Prefixed(jetstream.OutputClientData),
		durable:      cfg.Matrix.JetStream.Durable("UserAPIAccountDataConsumer"),
		db:           store,
		serverName:   cfg.Matrix.ServerName,
		syncProducer: syncProducer,
		pgClient:     pgClient,
	}
}

// Start consuming receipts events.
func (s *OutputClientDataConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, 1,
		s.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

func (s *OutputClientDataConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called

	userID := msg.Header.Get(jetstream.UserID)
	var output eventutil.AccountData
	if err := json.Unmarshal(msg.Data, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("client API server output log: message parse failure")
		sentry.CaptureException(err)
		return true
	}

	if output.Type != "m.fully_read" || output.ReadMarker == nil {
		return true
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithError(err).Error("userapi clientapi consumer: SplitID failure")
		return true
	}
	if domain != s.serverName {
		return true
	}

	log := log.WithFields(log.Fields{
		"room_id": output.RoomID,
		"user_id": userID,
	})

	_, serverName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return true
	}
	if serverName != s.serverName {
		return true
	}

	var notifyUsers bool
	if output.ReadMarker.Read != "" {
		_, err = s.db.SetNotificationsRead(ctx, localpart, output.RoomID, output.ReadMarker.Read, true)
		if err != nil {
			log.WithError(err).Error("userapi EDU consumer")
			return false
		}
		notifyUsers = true
	}

	if output.ReadMarker.FullyRead != "" {
		_, err := s.db.DeleteNotificationsUpTo(ctx, localpart, output.RoomID, output.ReadMarker.FullyRead)
		if err != nil {
			log.WithError(err).Errorf("userapi clientapi consumer: DeleteNotificationsUpTo failed")
			return false
		}
		notifyUsers = true
	}

	if !notifyUsers {
		return true
	}

	if err = s.syncProducer.GetAndSendNotificationData(ctx, userID, output.RoomID); err != nil {
		log.WithError(err).Error("userapi EDU consumer: GetAndSendNotificationData failed")
		return false
	}
	if err = util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
		log.WithError(err).Error("userapi EDU consumer: NotifyUserCounts failed")
		return false
	}

	return true
}

// OutputReceiptEventConsumer consumes events that originated in the EDU server.
type OutputReceiptEventConsumer struct {
	ctx          context.Context
	jetstream    nats.JetStreamContext
	durable      string
	topic        string
	db           storage.Database
	serverName   gomatrixserverlib.ServerName
	syncProducer *producers.SyncAPI
	pgClient     pushgateway.Client
}

// NewOutputReceiptEventConsumer creates a new OutputReceiptEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputReceiptEventConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.Database,
	syncProducer *producers.SyncAPI,
	pgClient pushgateway.Client,
) *OutputReceiptEventConsumer {
	return &OutputReceiptEventConsumer{
		ctx:          process.Context(),
		jetstream:    js,
		topic:        cfg.Matrix.JetStream.Prefixed(jetstream.OutputReceiptEvent),
		durable:      cfg.Matrix.JetStream.Durable("UserAPIReceiptConsumer"),
		db:           store,
		serverName:   cfg.Matrix.ServerName,
		syncProducer: syncProducer,
		pgClient:     pgClient,
	}
}

// Start consuming receipts events.
func (s *OutputReceiptEventConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, 1,
		s.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

func (s *OutputReceiptEventConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called

	userID := msg.Header.Get(jetstream.UserID)
	roomID := msg.Header.Get(jetstream.RoomID)
	readPos := msg.Header.Get(jetstream.EventID)
	evType := msg.Header.Get("type")

	if readPos == "" || evType != "m.read" {
		return true
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithError(err).Error("userapi clientapi consumer: SplitID failure")
		return true
	}
	if domain != s.serverName {
		return true
	}

	log := log.WithFields(log.Fields{
		"room_id": roomID,
		"user_id": userID,
	})

	_, err = s.db.SetNotificationsRead(ctx, localpart, roomID, readPos, true)
	if err != nil {
		log.WithError(err).Error("userapi EDU consumer")
		return false
	}

	if err = s.syncProducer.GetAndSendNotificationData(ctx, userID, roomID); err != nil {
		log.WithError(err).Error("userapi EDU consumer: GetAndSendNotificationData failed")
		return false
	}
	if err = util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
		log.WithError(err).Error("userapi EDU consumer: NotifyUserCounts failed")
		return false
	}

	return true
}
