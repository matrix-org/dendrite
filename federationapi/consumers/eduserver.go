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
	"strconv"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputEDUConsumer consumes events that originate in EDU server.
type OutputEDUConsumer struct {
	ctx               context.Context
	jetstream         nats.JetStreamContext
	durable           string
	db                storage.Database
	queues            *queue.OutgoingQueues
	ServerName        gomatrixserverlib.ServerName
	typingTopic       string
	sendToDeviceTopic string
	receiptTopic      string
}

// NewOutputEDUConsumer creates a new OutputEDUConsumer. Call Start() to begin consuming from EDU servers.
func NewOutputEDUConsumer(
	process *process.ProcessContext,
	cfg *config.FederationAPI,
	js nats.JetStreamContext,
	queues *queue.OutgoingQueues,
	store storage.Database,
) *OutputEDUConsumer {
	return &OutputEDUConsumer{
		ctx:               process.Context(),
		jetstream:         js,
		queues:            queues,
		db:                store,
		ServerName:        cfg.Matrix.ServerName,
		durable:           cfg.Matrix.JetStream.Durable("FederationAPIEDUServerConsumer"),
		typingTopic:       cfg.Matrix.JetStream.TopicFor(jetstream.OutputTypingEvent),
		sendToDeviceTopic: cfg.Matrix.JetStream.TopicFor(jetstream.OutputSendToDeviceEvent),
		receiptTopic:      cfg.Matrix.JetStream.TopicFor(jetstream.OutputReceiptEvent),
	}
}

// Start consuming from EDU servers
func (t *OutputEDUConsumer) Start() error {
	if err := jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.typingTopic, t.durable, t.onTypingEvent,
		nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	if err := jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.sendToDeviceTopic, t.durable, t.onSendToDeviceEvent,
		nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	if err := jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.receiptTopic, t.durable, t.onReceiptEvent,
		nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	return nil
}

// onSendToDeviceEvent is called in response to a message received on the
// send-to-device events topic from the EDU server.
func (t *OutputEDUConsumer) onSendToDeviceEvent(ctx context.Context, msg *nats.Msg) bool {
	sender := msg.Header.Get("sender")
	// only send send-to-device events which originated from us
	_, originServerName, err := gomatrixserverlib.SplitID('@', sender)
	if err != nil {
		log.WithError(err).WithField("user_id", sender).Error("Failed to extract domain from send-to-device sender")
		return true
	}
	if originServerName != t.ServerName {
		log.WithField("other_server", originServerName).Info("Suppressing send-to-device: originated elsewhere")
		return true
	}
	// Extract the send-to-device event from msg.
	var ote api.OutputSendToDeviceEvent
	if err := json.Unmarshal(msg.Data, &ote); err != nil {
		log.WithError(err).Errorf("eduserver output log: message parse failed (expected send-to-device)")
		return true
	}

	_, destServerName, err := gomatrixserverlib.SplitID('@', ote.UserID)
	if err != nil {
		log.WithError(err).WithField("user_id", ote.UserID).Error("Failed to extract domain from send-to-device destination")
		return true
	}

	// Pack the EDU and marshal it
	edu := &gomatrixserverlib.EDU{
		Type:   gomatrixserverlib.MDirectToDevice,
		Origin: string(t.ServerName),
	}
	tdm := gomatrixserverlib.ToDeviceMessage{
		Sender:    ote.Sender,
		Type:      ote.Type,
		MessageID: util.RandomString(32),
		Messages: map[string]map[string]json.RawMessage{
			ote.UserID: {
				ote.DeviceID: ote.Content,
			},
		},
	}
	if edu.Content, err = json.Marshal(tdm); err != nil {
		log.WithError(err).Error("failed to marshal EDU JSON")
		return true
	}

	log.Debugf("Sending send-to-device message into %q destination queue", destServerName)
	if err := t.queues.SendEDU(edu, t.ServerName, []gomatrixserverlib.ServerName{destServerName}); err != nil {
		log.WithError(err).Error("failed to send EDU")
		return false
	}

	return true
}

// onTypingEvent is called in response to a message received on the typing
// events topic from the EDU server.
func (t *OutputEDUConsumer) onTypingEvent(ctx context.Context, msg *nats.Msg) bool {
	// Extract the typing event from msg.

	roomID := msg.Header.Get(jetstream.RoomID)
	userID := msg.Header.Get(jetstream.UserID)
	typing, err := strconv.ParseBool(msg.Header.Get("typing"))
	if err != nil {
		log.WithError(err).Errorf("EDU server output log: typing parse failure")
		return true
	}

	// only send typing events which originated from us
	_, typingServerName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithError(err).WithField("user_id", userID).Error("Failed to extract domain from typing sender")
		_ = msg.Ack()
		return true
	}
	if typingServerName != t.ServerName {
		return true
	}

	joined, err := t.db.GetJoinedHosts(ctx, roomID)
	if err != nil {
		log.WithError(err).WithField("room_id", roomID).Error("failed to get joined hosts for room")
		return false
	}

	names := make([]gomatrixserverlib.ServerName, len(joined))
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
	log.Debugf("sending edu: %+v", edu)
	if err := t.queues.SendEDU(edu, t.ServerName, names); err != nil {
		log.WithError(err).Error("failed to send EDU")
		return false
	}

	return true
}

// onReceiptEvent is called in response to a message received on the receipt
// events topic from the EDU server.
func (t *OutputEDUConsumer) onReceiptEvent(ctx context.Context, msg *nats.Msg) bool {
	receipt := api.OutputReceiptEvent{
		UserID:  msg.Header.Get(jetstream.UserID),
		RoomID:  msg.Header.Get(jetstream.RoomID),
		EventID: msg.Header.Get(jetstream.EventID),
		Type:    msg.Header.Get("type"),
	}

	timestamp, err := strconv.Atoi(msg.Header.Get("timestamp"))
	if err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("EDU server output log: message parse failure")
		sentry.CaptureException(err)
		return true
	}

	receipt.Timestamp = gomatrixserverlib.Timestamp(timestamp)

	// only send receipt events which originated from us
	// TODO: We're consuming/producing on the same topic from the federation api, maybe add different topics?
	_, receiptServerName, err := gomatrixserverlib.SplitID('@', receipt.UserID)
	if err != nil {
		log.WithError(err).WithField("user_id", receipt.UserID).Error("failed to extract domain from receipt sender")
		return true
	}
	if receiptServerName != t.ServerName {
		return true
	}

	joined, err := t.db.GetJoinedHosts(ctx, receipt.RoomID)
	if err != nil {
		log.WithError(err).WithField("room_id", receipt.RoomID).Error("failed to get joined hosts for room")
		return false
	}

	names := make([]gomatrixserverlib.ServerName, len(joined))
	for i := range joined {
		names[i] = joined[i].ServerName
	}

	content := map[string]api.FederationReceiptMRead{}
	content[receipt.RoomID] = api.FederationReceiptMRead{
		User: map[string]api.FederationReceiptData{
			receipt.UserID: {
				Data: api.ReceiptTS{
					TS: receipt.Timestamp,
				},
				EventIDs: []string{receipt.EventID},
			},
		},
	}

	edu := &gomatrixserverlib.EDU{
		Type:   gomatrixserverlib.MReceipt,
		Origin: string(t.ServerName),
	}
	if edu.Content, err = json.Marshal(content); err != nil {
		log.WithError(err).Error("failed to marshal EDU JSON")
		return true
	}

	if err := t.queues.SendEDU(edu, t.ServerName, names); err != nil {
		log.WithError(err).Error("failed to send EDU")
		return false
	}

	return true
}
