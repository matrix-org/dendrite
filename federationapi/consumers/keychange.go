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

	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// KeyChangeConsumer consumes events that originate in key server.
type KeyChangeConsumer struct {
	ctx        context.Context
	jetstream  nats.JetStreamContext
	durable    string
	db         storage.Database
	queues     *queue.OutgoingQueues
	serverName gomatrixserverlib.ServerName
	rsAPI      roomserverAPI.RoomserverInternalAPI
	topic      string
}

// NewKeyChangeConsumer creates a new KeyChangeConsumer. Call Start() to begin consuming from key servers.
func NewKeyChangeConsumer(
	process *process.ProcessContext,
	cfg *config.KeyServer,
	js nats.JetStreamContext,
	queues *queue.OutgoingQueues,
	store storage.Database,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) *KeyChangeConsumer {
	return &KeyChangeConsumer{
		ctx:        process.Context(),
		jetstream:  js,
		durable:    cfg.Matrix.JetStream.Prefixed("FederationAPIKeyChangeConsumer"),
		topic:      cfg.Matrix.JetStream.Prefixed(jetstream.OutputKeyChangeEvent),
		queues:     queues,
		db:         store,
		serverName: cfg.Matrix.ServerName,
		rsAPI:      rsAPI,
	}
}

// Start consuming from key servers
func (t *KeyChangeConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.topic, t.durable, t.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called in response to a message received on the
// key change events topic from the key server.
func (t *KeyChangeConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	var m api.DeviceMessage
	if err := json.Unmarshal(msg.Data, &m); err != nil {
		logrus.WithError(err).Errorf("failed to read device message from key change topic")
		return true
	}
	if m.DeviceKeys == nil && m.OutputCrossSigningKeyUpdate == nil {
		// This probably shouldn't happen but stops us from panicking if we come
		// across an update that doesn't satisfy either types.
		return true
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

func (t *KeyChangeConsumer) onDeviceKeyMessage(m api.DeviceMessage) bool {
	if m.DeviceKeys == nil {
		return true
	}
	logger := logrus.WithField("user_id", m.UserID)

	// only send key change events which originated from us
	_, originServerName, err := gomatrixserverlib.SplitID('@', m.UserID)
	if err != nil {
		logger.WithError(err).Error("Failed to extract domain from key change event")
		return true
	}
	if originServerName != t.serverName {
		return true
	}

	var queryRes roomserverAPI.QueryRoomsForUserResponse
	err = t.rsAPI.QueryRoomsForUser(t.ctx, &roomserverAPI.QueryRoomsForUserRequest{
		UserID:         m.UserID,
		WantMembership: "join",
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined rooms for user")
		return true
	}
	// send this key change to all servers who share rooms with this user.
	destinations, err := t.db.GetJoinedHostsForRooms(t.ctx, queryRes.RoomIDs, true)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined hosts for rooms user is in")
		return true
	}

	if len(destinations) == 0 {
		return true
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
		logger.WithError(err).Error("failed to marshal EDU JSON")
		return true
	}

	logger.Debugf("Sending device list update message to %q", destinations)
	err = t.queues.SendEDU(edu, t.serverName, destinations)
	return err == nil
}

func (t *KeyChangeConsumer) onCrossSigningMessage(m api.DeviceMessage) bool {
	output := m.CrossSigningKeyUpdate
	_, host, err := gomatrixserverlib.SplitID('@', output.UserID)
	if err != nil {
		logrus.WithError(err).Errorf("fedsender key change consumer: user ID parse failure")
		return true
	}
	if host != gomatrixserverlib.ServerName(t.serverName) {
		// Ignore any messages that didn't originate locally, otherwise we'll
		// end up parroting information we received from other servers.
		return true
	}
	logger := logrus.WithField("user_id", output.UserID)

	var queryRes roomserverAPI.QueryRoomsForUserResponse
	err = t.rsAPI.QueryRoomsForUser(t.ctx, &roomserverAPI.QueryRoomsForUserRequest{
		UserID:         output.UserID,
		WantMembership: "join",
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("fedsender key change consumer: failed to calculate joined rooms for user")
		return true
	}
	// send this key change to all servers who share rooms with this user.
	destinations, err := t.db.GetJoinedHostsForRooms(t.ctx, queryRes.RoomIDs, true)
	if err != nil {
		logger.WithError(err).Error("fedsender key change consumer: failed to calculate joined hosts for rooms user is in")
		return true
	}

	if len(destinations) == 0 {
		return true
	}

	// Pack the EDU and marshal it
	edu := &gomatrixserverlib.EDU{
		Type:   types.MSigningKeyUpdate,
		Origin: string(t.serverName),
	}
	if edu.Content, err = json.Marshal(output); err != nil {
		logger.WithError(err).Error("fedsender key change consumer: failed to marshal output, dropping")
		return true
	}

	logger.Debugf("Sending cross-signing update message to %q", destinations)
	err = t.queues.SendEDU(edu, t.serverName, destinations)
	return err == nil
}

func prevID(streamID int64) []int64 {
	if streamID <= 1 {
		return nil
	}
	return []int64{streamID - 1}
}
