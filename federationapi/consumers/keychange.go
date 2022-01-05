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
	eduserverAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// KeyChangeConsumer consumes events that originate in key server.
type KeyChangeConsumer struct {
	ctx        context.Context
	consumer   *internal.ContinualConsumer
	db         storage.Database
	queues     *queue.OutgoingQueues
	serverName gomatrixserverlib.ServerName
	rsAPI      roomserverAPI.RoomserverInternalAPI
}

// NewKeyChangeConsumer creates a new KeyChangeConsumer. Call Start() to begin consuming from key servers.
func NewKeyChangeConsumer(
	process *process.ProcessContext,
	cfg *config.KeyServer,
	kafkaConsumer sarama.Consumer,
	queues *queue.OutgoingQueues,
	store storage.Database,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) *KeyChangeConsumer {
	c := &KeyChangeConsumer{
		ctx: process.Context(),
		consumer: &internal.ContinualConsumer{
			Process:        process,
			ComponentName:  "federationapi/keychange",
			Topic:          string(cfg.Matrix.JetStream.TopicFor(jetstream.OutputKeyChangeEvent)),
			Consumer:       kafkaConsumer,
			PartitionStore: store,
		},
		queues:     queues,
		db:         store,
		serverName: cfg.Matrix.ServerName,
		rsAPI:      rsAPI,
	}
	c.consumer.ProcessMessage = c.onMessage

	return c
}

// Start consuming from key servers
func (t *KeyChangeConsumer) Start() error {
	if err := t.consumer.Start(); err != nil {
		return fmt.Errorf("t.consumer.Start: %w", err)
	}
	return nil
}

// onMessage is called in response to a message received on the
// key change events topic from the key server.
func (t *KeyChangeConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var m api.DeviceMessage
	if err := json.Unmarshal(msg.Value, &m); err != nil {
		logrus.WithError(err).Errorf("failed to read device message from key change topic")
		return nil
	}
	if m.DeviceKeys == nil && m.OutputCrossSigningKeyUpdate == nil {
		// This probably shouldn't happen but stops us from panicking if we come
		// across an update that doesn't satisfy either types.
		return nil
	}
	switch m.Type {
	case api.TypeCrossSigningUpdate:
		return t.onCrossSigningMessage(m)
	case api.TypeDeviceKeyUpdate:
		fallthrough
	default:
		return t.onDeviceKeyMessage(m)
	}
}

func (t *KeyChangeConsumer) onDeviceKeyMessage(m api.DeviceMessage) error {
	if m.DeviceKeys == nil {
		return nil
	}
	logger := logrus.WithField("user_id", m.UserID)

	// only send key change events which originated from us
	_, originServerName, err := gomatrixserverlib.SplitID('@', m.UserID)
	if err != nil {
		logger.WithError(err).Error("Failed to extract domain from key change event")
		return nil
	}
	if originServerName != t.serverName {
		return nil
	}

	var queryRes roomserverAPI.QueryRoomsForUserResponse
	err = t.rsAPI.QueryRoomsForUser(t.ctx, &roomserverAPI.QueryRoomsForUserRequest{
		UserID:         m.UserID,
		WantMembership: "join",
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined rooms for user")
		return nil
	}
	// send this key change to all servers who share rooms with this user.
	destinations, err := t.db.GetJoinedHostsForRooms(t.ctx, queryRes.RoomIDs)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined hosts for rooms user is in")
		return nil
	}

	// Pack the EDU and marshal it
	edu := &gomatrixserverlib.EDU{
		Type:   gomatrixserverlib.MDeviceListUpdate,
		Origin: string(t.serverName),
	}
	event := gomatrixserverlib.DeviceListUpdateEvent{
		UserID:            m.UserID,
		DeviceID:          m.DeviceID,
		DeviceDisplayName: m.DisplayName,
		StreamID:          m.StreamID,
		PrevID:            prevID(m.StreamID),
		Deleted:           len(m.KeyJSON) == 0,
		Keys:              m.KeyJSON,
	}
	if edu.Content, err = json.Marshal(event); err != nil {
		return err
	}

	logrus.Infof("Sending device list update message to %q", destinations)
	return t.queues.SendEDU(edu, t.serverName, destinations)
}

func (t *KeyChangeConsumer) onCrossSigningMessage(m api.DeviceMessage) error {
	output := m.CrossSigningKeyUpdate
	_, host, err := gomatrixserverlib.SplitID('@', output.UserID)
	if err != nil {
		logrus.WithError(err).Errorf("fedsender key change consumer: user ID parse failure")
		return nil
	}
	if host != gomatrixserverlib.ServerName(t.serverName) {
		// Ignore any messages that didn't originate locally, otherwise we'll
		// end up parroting information we received from other servers.
		return nil
	}
	logger := logrus.WithField("user_id", output.UserID)

	var queryRes roomserverAPI.QueryRoomsForUserResponse
	err = t.rsAPI.QueryRoomsForUser(t.ctx, &roomserverAPI.QueryRoomsForUserRequest{
		UserID:         output.UserID,
		WantMembership: "join",
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("fedsender key change consumer: failed to calculate joined rooms for user")
		return nil
	}
	// send this key change to all servers who share rooms with this user.
	destinations, err := t.db.GetJoinedHostsForRooms(t.ctx, queryRes.RoomIDs)
	if err != nil {
		logger.WithError(err).Error("fedsender key change consumer: failed to calculate joined hosts for rooms user is in")
		return nil
	}

	// Pack the EDU and marshal it
	edu := &gomatrixserverlib.EDU{
		Type:   eduserverAPI.MSigningKeyUpdate,
		Origin: string(t.serverName),
	}
	if edu.Content, err = json.Marshal(output); err != nil {
		logger.WithError(err).Error("fedsender key change consumer: failed to marshal output, dropping")
		return nil
	}

	logger.Infof("Sending cross-signing update message to %q", destinations)
	return t.queues.SendEDU(edu, t.serverName, destinations)
}

func prevID(streamID int) []int {
	if streamID <= 1 {
		return nil
	}
	return []int{streamID - 1}
}
