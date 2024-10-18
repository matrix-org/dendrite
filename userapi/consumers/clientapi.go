// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/internal/pushgateway"
	"github.com/element-hq/dendrite/userapi/storage"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/userapi/producers"
	"github.com/element-hq/dendrite/userapi/util"
)

// OutputReceiptEventConsumer consumes events that originated in the clientAPI.
type OutputReceiptEventConsumer struct {
	ctx          context.Context
	jetstream    nats.JetStreamContext
	durable      string
	topic        string
	db           storage.UserDatabase
	serverName   spec.ServerName
	syncProducer *producers.SyncAPI
	pgClient     pushgateway.Client
}

// NewOutputReceiptEventConsumer creates a new OutputReceiptEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputReceiptEventConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.UserDatabase,
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

	if readPos == "" || (evType != "m.read" && evType != "m.read.private") {
		return true
	}

	log := log.WithFields(log.Fields{
		"room_id": roomID,
		"user_id": userID,
	})

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithError(err).Error("userapi clientapi consumer: SplitID failure")
		return true
	}
	if domain != s.serverName {
		return true
	}

	metadata, err := msg.Metadata()
	if err != nil {
		return false
	}

	updated, err := s.db.SetNotificationsRead(ctx, localpart, domain, roomID, uint64(spec.AsTimestamp(metadata.Timestamp)), true)
	if err != nil {
		log.WithError(err).Error("userapi EDU consumer")
		return false
	}

	if err = s.syncProducer.GetAndSendNotificationData(ctx, userID, roomID); err != nil {
		log.WithError(err).Error("userapi EDU consumer: GetAndSendNotificationData failed")
		return false
	}

	if !updated {
		return true
	}
	if err = util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, domain, s.db); err != nil {
		log.WithError(err).Error("userapi EDU consumer: NotifyUserCounts failed")
		return false
	}

	return true
}
