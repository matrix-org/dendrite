// Copyright 2021 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type OutputCrossSigningKeyUpdateConsumer struct {
	ctx        context.Context
	keyDB      storage.Database
	keyAPI     api.KeyInternalAPI
	serverName string
	jetstream  nats.JetStreamContext
	durable    string
	topic      string
}

func NewOutputCrossSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.Dendrite,
	js nats.JetStreamContext,
	keyDB storage.Database,
	keyAPI api.KeyInternalAPI,
) *OutputCrossSigningKeyUpdateConsumer {
	// The keyserver both produces and consumes on the TopicOutputKeyChangeEvent
	// topic. We will only produce events where the UserID matches our server name,
	// and we will only consume events where the UserID does NOT match our server
	// name (because the update came from a remote server).
	s := &OutputCrossSigningKeyUpdateConsumer{
		ctx:        process.Context(),
		keyDB:      keyDB,
		jetstream:  js,
		durable:    cfg.Global.JetStream.Durable("KeyServerCrossSigningConsumer"),
		topic:      cfg.Global.JetStream.TopicFor(jetstream.OutputKeyChangeEvent),
		keyAPI:     keyAPI,
		serverName: string(cfg.Global.ServerName),
	}

	return s
}

func (s *OutputCrossSigningKeyUpdateConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called in response to a message received on the
// key change events topic from the key server.
func (t *OutputCrossSigningKeyUpdateConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	var m api.DeviceMessage
	if err := json.Unmarshal(msg.Data, &m); err != nil {
		logrus.WithError(err).Errorf("failed to read device message from key change topic")
		return true
	}
	if m.OutputCrossSigningKeyUpdate == nil {
		// This probably shouldn't happen but stops us from panicking if we come
		// across an update that doesn't satisfy either types.
		return true
	}
	switch m.Type {
	case api.TypeCrossSigningUpdate:
		return t.onCrossSigningMessage(m)
	default:
		return true
	}
}

func (s *OutputCrossSigningKeyUpdateConsumer) onCrossSigningMessage(m api.DeviceMessage) bool {
	output := m.CrossSigningKeyUpdate
	_, host, err := gomatrixserverlib.SplitID('@', output.UserID)
	if err != nil {
		logrus.WithError(err).Errorf("eduserver output log: user ID parse failure")
		return true
	}
	if host == gomatrixserverlib.ServerName(s.serverName) {
		// Ignore any messages that contain information about our own users, as
		// they already originated from this server.
		return true
	}
	uploadReq := &api.PerformUploadDeviceKeysRequest{
		UserID: output.UserID,
	}
	if output.MasterKey != nil {
		uploadReq.MasterKey = *output.MasterKey
	}
	if output.SelfSigningKey != nil {
		uploadReq.SelfSigningKey = *output.SelfSigningKey
	}
	uploadRes := &api.PerformUploadDeviceKeysResponse{}
	s.keyAPI.PerformUploadDeviceKeys(context.TODO(), uploadReq, uploadRes)
	if uploadRes.Error != nil {
		// If the error is due to a missing or invalid parameter then we'd might
		// as well just acknowledge the message, because otherwise otherwise we'll
		// just keep getting delivered a faulty message over and over again.
		return uploadRes.Error.IsMissingParam || uploadRes.Error.IsInvalidParam
	}
	return true
}
