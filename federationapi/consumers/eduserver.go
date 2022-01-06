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
		typingTopic:       cfg.Matrix.JetStream.TopicFor(jetstream.OutputTypingEvent),
		sendToDeviceTopic: cfg.Matrix.JetStream.TopicFor(jetstream.OutputSendToDeviceEvent),
		receiptTopic:      cfg.Matrix.JetStream.TopicFor(jetstream.OutputReceiptEvent),
	}
}

// Start consuming from EDU servers
func (t *OutputEDUConsumer) Start() error {
	if _, err := t.jetstream.Subscribe(t.typingTopic, t.onTypingEvent); err != nil {
		return err
	}
	if _, err := t.jetstream.Subscribe(t.sendToDeviceTopic, t.onSendToDeviceEvent); err != nil {
		return err
	}
	if _, err := t.jetstream.Subscribe(t.receiptTopic, t.onReceiptEvent); err != nil {
		return err
	}
	return nil
}

// onSendToDeviceEvent is called in response to a message received on the
// send-to-device events topic from the EDU server.
func (t *OutputEDUConsumer) onSendToDeviceEvent(msg *nats.Msg) {
	// Extract the send-to-device event from msg.
	jetstream.WithJetStreamMessage(msg, func(msg *nats.Msg) bool {
		var ote api.OutputSendToDeviceEvent
		if err := json.Unmarshal(msg.Data, &ote); err != nil {
			log.WithError(err).Errorf("eduserver output log: message parse failed (expected send-to-device)")
			return true
		}

		// only send send-to-device events which originated from us
		_, originServerName, err := gomatrixserverlib.SplitID('@', ote.Sender)
		if err != nil {
			log.WithError(err).WithField("user_id", ote.Sender).Error("Failed to extract domain from send-to-device sender")
			return true
		}
		if originServerName != t.ServerName {
			log.WithField("other_server", originServerName).Info("Suppressing send-to-device: originated elsewhere")
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

		log.Infof("Sending send-to-device message into %q destination queue", destServerName)
		if err := t.queues.SendEDU(edu, t.ServerName, []gomatrixserverlib.ServerName{destServerName}); err != nil {
			log.WithError(err).Error("failed to send EDU")
			return false
		}

		return true
	})
}

// onTypingEvent is called in response to a message received on the typing
// events topic from the EDU server.
func (t *OutputEDUConsumer) onTypingEvent(msg *nats.Msg) {
	jetstream.WithJetStreamMessage(msg, func(msg *nats.Msg) bool {
		// Extract the typing event from msg.
		var ote api.OutputTypingEvent
		if err := json.Unmarshal(msg.Data, &ote); err != nil {
			// Skip this msg but continue processing messages.
			log.WithError(err).Errorf("eduserver output log: message parse failed (expected typing)")
			_ = msg.Ack()
			return true
		}

		// only send typing events which originated from us
		_, typingServerName, err := gomatrixserverlib.SplitID('@', ote.Event.UserID)
		if err != nil {
			log.WithError(err).WithField("user_id", ote.Event.UserID).Error("Failed to extract domain from typing sender")
			_ = msg.Ack()
			return true
		}
		if typingServerName != t.ServerName {
			return true
		}

		joined, err := t.db.GetJoinedHosts(t.ctx, ote.Event.RoomID)
		if err != nil {
			log.WithError(err).WithField("room_id", ote.Event.RoomID).Error("failed to get joined hosts for room")
			return false
		}

		names := make([]gomatrixserverlib.ServerName, len(joined))
		for i := range joined {
			names[i] = joined[i].ServerName
		}

		edu := &gomatrixserverlib.EDU{Type: ote.Event.Type}
		if edu.Content, err = json.Marshal(map[string]interface{}{
			"room_id": ote.Event.RoomID,
			"user_id": ote.Event.UserID,
			"typing":  ote.Event.Typing,
		}); err != nil {
			log.WithError(err).Error("failed to marshal EDU JSON")
			return true
		}

		if err := t.queues.SendEDU(edu, t.ServerName, names); err != nil {
			log.WithError(err).Error("failed to send EDU")
			return false
		}

		return true
	})
}

// onReceiptEvent is called in response to a message received on the receipt
// events topic from the EDU server.
func (t *OutputEDUConsumer) onReceiptEvent(msg *nats.Msg) {
	jetstream.WithJetStreamMessage(msg, func(msg *nats.Msg) bool {
		// Extract the typing event from msg.
		var receipt api.OutputReceiptEvent
		if err := json.Unmarshal(msg.Data, &receipt); err != nil {
			// Skip this msg but continue processing messages.
			log.WithError(err).Errorf("eduserver output log: message parse failed (expected receipt)")
			return true
		}

		// only send receipt events which originated from us
		_, receiptServerName, err := gomatrixserverlib.SplitID('@', receipt.UserID)
		if err != nil {
			log.WithError(err).WithField("user_id", receipt.UserID).Error("failed to extract domain from receipt sender")
			return true
		}
		if receiptServerName != t.ServerName {
			return true
		}

		joined, err := t.db.GetJoinedHosts(t.ctx, receipt.RoomID)
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
	})
}
