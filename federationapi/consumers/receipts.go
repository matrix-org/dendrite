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
	"strconv"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/storage"
	fedTypes "github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	syncTypes "github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputReceiptConsumer consumes events that originate in the clientapi.
type OutputReceiptConsumer struct {
	ctx               context.Context
	jetstream         nats.JetStreamContext
	durable           string
	db                storage.Database
	queues            *queue.OutgoingQueues
	isLocalServerName func(gomatrixserverlib.ServerName) bool
	topic             string
}

// NewOutputReceiptConsumer creates a new OutputReceiptConsumer. Call Start() to begin consuming typing events.
func NewOutputReceiptConsumer(
	process *process.ProcessContext,
	cfg *config.FederationAPI,
	js nats.JetStreamContext,
	queues *queue.OutgoingQueues,
	store storage.Database,
) *OutputReceiptConsumer {
	return &OutputReceiptConsumer{
		ctx:               process.Context(),
		jetstream:         js,
		queues:            queues,
		db:                store,
		isLocalServerName: cfg.Matrix.IsLocalServerName,
		durable:           cfg.Matrix.JetStream.Durable("FederationAPIReceiptConsumer"),
		topic:             cfg.Matrix.JetStream.Prefixed(jetstream.OutputReceiptEvent),
	}
}

// Start consuming from the clientapi
func (t *OutputReceiptConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.topic, t.durable, 1, t.onMessage,
		nats.DeliverAll(), nats.ManualAck(), nats.HeadersOnly(),
	)
}

// onMessage is called in response to a message received on the receipt
// events topic from the client api.
func (t *OutputReceiptConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	receipt := syncTypes.OutputReceiptEvent{
		UserID:  msg.Header.Get(jetstream.UserID),
		RoomID:  msg.Header.Get(jetstream.RoomID),
		EventID: msg.Header.Get(jetstream.EventID),
		Type:    msg.Header.Get("type"),
	}

	switch receipt.Type {
	case "m.read":
		// These are allowed to be sent over federation
	case "m.read.private", "m.fully_read":
		// These must not be sent over federation
		return true
	}

	// only send receipt events which originated from us
	_, receiptServerName, err := gomatrixserverlib.SplitID('@', receipt.UserID)
	if err != nil {
		log.WithError(err).WithField("user_id", receipt.UserID).Error("failed to extract domain from receipt sender")
		return true
	}
	if !t.isLocalServerName(receiptServerName) {
		return true
	}

	timestamp, err := strconv.ParseUint(msg.Header.Get("timestamp"), 10, 64)
	if err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("EDU output log: message parse failure")
		sentry.CaptureException(err)
		return true
	}

	receipt.Timestamp = gomatrixserverlib.Timestamp(timestamp)

	joined, err := t.db.GetJoinedHosts(ctx, receipt.RoomID)
	if err != nil {
		log.WithError(err).WithField("room_id", receipt.RoomID).Error("failed to get joined hosts for room")
		return false
	}

	names := make([]gomatrixserverlib.ServerName, len(joined))
	for i := range joined {
		names[i] = joined[i].ServerName
	}

	content := map[string]fedTypes.FederationReceiptMRead{}
	content[receipt.RoomID] = fedTypes.FederationReceiptMRead{
		User: map[string]fedTypes.FederationReceiptData{
			receipt.UserID: {
				Data: fedTypes.ReceiptTS{
					TS: receipt.Timestamp,
				},
				EventIDs: []string{receipt.EventID},
			},
		},
	}

	edu := &gomatrixserverlib.EDU{
		Type:   gomatrixserverlib.MReceipt,
		Origin: string(receiptServerName),
	}
	if edu.Content, err = json.Marshal(content); err != nil {
		log.WithError(err).Error("failed to marshal EDU JSON")
		return true
	}

	if err := t.queues.SendEDU(edu, receiptServerName, names); err != nil {
		log.WithError(err).Error("failed to send EDU")
		return false
	}

	return true
}
