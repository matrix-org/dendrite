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

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// OutputEDUConsumer consumes events that originate in EDU server.
type OutputEDUConsumer struct {
	typingConsumer       *internal.ContinualConsumer
	sendToDeviceConsumer *internal.ContinualConsumer
	db                   storage.Database
	queues               *queue.OutgoingQueues
	ServerName           gomatrixserverlib.ServerName
	TypingTopic          string
	SendToDeviceTopic    string
}

// NewOutputEDUConsumer creates a new OutputEDUConsumer. Call Start() to begin consuming from EDU servers.
func NewOutputEDUConsumer(
	cfg *config.FederationSender,
	kafkaConsumer sarama.Consumer,
	queues *queue.OutgoingQueues,
	store storage.Database,
) *OutputEDUConsumer {
	c := &OutputEDUConsumer{
		typingConsumer: &internal.ContinualConsumer{
			ComponentName:  "eduserver/typing",
			Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputTypingEvent)),
			Consumer:       kafkaConsumer,
			PartitionStore: store,
		},
		sendToDeviceConsumer: &internal.ContinualConsumer{
			ComponentName:  "eduserver/sendtodevice",
			Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputSendToDeviceEvent)),
			Consumer:       kafkaConsumer,
			PartitionStore: store,
		},
		queues:            queues,
		db:                store,
		ServerName:        cfg.Matrix.ServerName,
		TypingTopic:       string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputTypingEvent)),
		SendToDeviceTopic: string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputSendToDeviceEvent)),
	}
	c.typingConsumer.ProcessMessage = c.onTypingEvent
	c.sendToDeviceConsumer.ProcessMessage = c.onSendToDeviceEvent

	return c
}

// Start consuming from EDU servers
func (t *OutputEDUConsumer) Start() error {
	if err := t.typingConsumer.Start(); err != nil {
		return fmt.Errorf("t.typingConsumer.Start: %w", err)
	}
	if err := t.sendToDeviceConsumer.Start(); err != nil {
		return fmt.Errorf("t.sendToDeviceConsumer.Start: %w", err)
	}
	return nil
}

// onSendToDeviceEvent is called in response to a message received on the
// send-to-device events topic from the EDU server.
func (t *OutputEDUConsumer) onSendToDeviceEvent(msg *sarama.ConsumerMessage) error {
	// Extract the send-to-device event from msg.
	var ote api.OutputSendToDeviceEvent
	if err := json.Unmarshal(msg.Value, &ote); err != nil {
		log.WithError(err).Errorf("eduserver output log: message parse failed (expected send-to-device)")
		return nil
	}

	// only send send-to-device events which originated from us
	_, originServerName, err := gomatrixserverlib.SplitID('@', ote.Sender)
	if err != nil {
		log.WithError(err).WithField("user_id", ote.Sender).Error("Failed to extract domain from send-to-device sender")
		return nil
	}
	if originServerName != t.ServerName {
		log.WithField("other_server", originServerName).Info("Suppressing send-to-device: originated elsewhere")
		return nil
	}

	_, destServerName, err := gomatrixserverlib.SplitID('@', ote.UserID)
	if err != nil {
		log.WithError(err).WithField("user_id", ote.UserID).Error("Failed to extract domain from send-to-device destination")
		return nil
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
		return err
	}

	log.Infof("Sending send-to-device message into %q destination queue", destServerName)
	return t.queues.SendEDU(edu, t.ServerName, []gomatrixserverlib.ServerName{destServerName})
}

// onTypingEvent is called in response to a message received on the typing
// events topic from the EDU server.
func (t *OutputEDUConsumer) onTypingEvent(msg *sarama.ConsumerMessage) error {
	// Extract the typing event from msg.
	var ote api.OutputTypingEvent
	if err := json.Unmarshal(msg.Value, &ote); err != nil {
		// Skip this msg but continue processing messages.
		log.WithError(err).Errorf("eduserver output log: message parse failed (expected typing)")
		return nil
	}

	// only send typing events which originated from us
	_, typingServerName, err := gomatrixserverlib.SplitID('@', ote.Event.UserID)
	if err != nil {
		log.WithError(err).WithField("user_id", ote.Event.UserID).Error("Failed to extract domain from typing sender")
		return nil
	}
	if typingServerName != t.ServerName {
		log.WithField("other_server", typingServerName).Info("Suppressing typing notif: originated elsewhere")
		return nil
	}

	joined, err := t.db.GetJoinedHosts(context.TODO(), ote.Event.RoomID)
	if err != nil {
		return err
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
		return err
	}

	return t.queues.SendEDU(edu, t.ServerName, names)
}
