// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/element-hq/dendrite/federationapi/queue"
	"github.com/element-hq/dendrite/federationapi/storage"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputTypingConsumer consumes events that originate in the clientapi.
type OutputTypingConsumer struct {
	ctx               context.Context
	jetstream         nats.JetStreamContext
	durable           string
	db                storage.Database
	queues            *queue.OutgoingQueues
	isLocalServerName func(spec.ServerName) bool
	topic             string
}

// NewOutputTypingConsumer creates a new OutputTypingConsumer. Call Start() to begin consuming typing events.
func NewOutputTypingConsumer(
	process *process.ProcessContext,
	cfg *config.FederationAPI,
	js nats.JetStreamContext,
	queues *queue.OutgoingQueues,
	store storage.Database,
) *OutputTypingConsumer {
	return &OutputTypingConsumer{
		ctx:               process.Context(),
		jetstream:         js,
		queues:            queues,
		db:                store,
		isLocalServerName: cfg.Matrix.IsLocalServerName,
		durable:           cfg.Matrix.JetStream.Durable("FederationAPITypingConsumer"),
		topic:             cfg.Matrix.JetStream.Prefixed(jetstream.OutputTypingEvent),
	}
}

// Start consuming from the clientapi
func (t *OutputTypingConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.topic, t.durable, 1, t.onMessage,
		nats.DeliverAll(), nats.ManualAck(), nats.HeadersOnly(),
	)
}

// onMessage is called in response to a message received on the typing
// events topic from the client api.
func (t *OutputTypingConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	// Extract the typing event from msg.
	roomID := msg.Header.Get(jetstream.RoomID)
	userID := msg.Header.Get(jetstream.UserID)
	typing, err := strconv.ParseBool(msg.Header.Get("typing"))
	if err != nil {
		log.WithError(err).Errorf("EDU output log: typing parse failure")
		return true
	}

	// only send typing events which originated from us
	_, typingServerName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithError(err).WithField("user_id", userID).Error("Failed to extract domain from typing sender")
		_ = msg.Ack()
		return true
	}
	if !t.isLocalServerName(typingServerName) {
		return true
	}

	joined, err := t.db.GetJoinedHosts(ctx, roomID)
	if err != nil {
		log.WithError(err).WithField("room_id", roomID).Error("failed to get joined hosts for room")
		return false
	}

	names := make([]spec.ServerName, len(joined))
	for i := range joined {
		names[i] = joined[i].ServerName
	}

	edu := &gomatrixserverlib.EDU{Type: "m.typing"}
	if edu.Content, err = json.Marshal(map[string]interface{}{
		"room_id": roomID,
		"user_id": userID,
		"typing":  typing,
	}); err != nil {
		log.WithError(err).Error("failed to marshal EDU JSON")
		return true
	}
	if err := t.queues.SendEDU(edu, typingServerName, names); err != nil {
		log.WithError(err).Error("failed to send EDU")
		return false
	}

	return true
}
