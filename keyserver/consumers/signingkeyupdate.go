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

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
)

// SigningKeyUpdateConsumer consumes signing key updates that came in over federation.
type SigningKeyUpdateConsumer struct {
	ctx               context.Context
	jetstream         nats.JetStreamContext
	durable           string
	topic             string
	keyAPI            *internal.KeyInternalAPI
	cfg               *config.KeyServer
	isLocalServerName func(gomatrixserverlib.ServerName) bool
}

// NewSigningKeyUpdateConsumer creates a new SigningKeyUpdateConsumer. Call Start() to begin consuming from key servers.
func NewSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.KeyServer,
	js nats.JetStreamContext,
	keyAPI *internal.KeyInternalAPI,
) *SigningKeyUpdateConsumer {
	return &SigningKeyUpdateConsumer{
		ctx:               process.Context(),
		jetstream:         js,
		durable:           cfg.Matrix.JetStream.Prefixed("KeyServerSigningKeyConsumer"),
		topic:             cfg.Matrix.JetStream.Prefixed(jetstream.InputSigningKeyUpdate),
		keyAPI:            keyAPI,
		cfg:               cfg,
		isLocalServerName: cfg.Matrix.IsLocalServerName,
	}
}

// Start consuming from key servers
func (t *SigningKeyUpdateConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.topic, t.durable, 1,
		t.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called in response to a message received on the
// signing key update events topic from the key server.
func (t *SigningKeyUpdateConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	var updatePayload keyapi.CrossSigningKeyUpdate
	if err := json.Unmarshal(msg.Data, &updatePayload); err != nil {
		logrus.WithError(err).Errorf("Failed to read from signing key update input topic")
		return true
	}
	origin := gomatrixserverlib.ServerName(msg.Header.Get("origin"))
	if _, serverName, err := gomatrixserverlib.SplitID('@', updatePayload.UserID); err != nil {
		logrus.WithError(err).Error("failed to split user id")
		return true
	} else if t.isLocalServerName(serverName) {
		logrus.Warn("dropping device key update from ourself")
		return true
	} else if serverName != origin {
		logrus.Warnf("dropping device key update, %s != %s", serverName, origin)
		return true
	}

	keys := gomatrixserverlib.CrossSigningKeys{}
	if updatePayload.MasterKey != nil {
		keys.MasterKey = *updatePayload.MasterKey
	}
	if updatePayload.SelfSigningKey != nil {
		keys.SelfSigningKey = *updatePayload.SelfSigningKey
	}
	uploadReq := &keyapi.PerformUploadDeviceKeysRequest{
		CrossSigningKeys: keys,
		UserID:           updatePayload.UserID,
	}
	uploadRes := &keyapi.PerformUploadDeviceKeysResponse{}
	if err := t.keyAPI.PerformUploadDeviceKeys(ctx, uploadReq, uploadRes); err != nil {
		logrus.WithError(err).Error("failed to upload device keys")
		return false
	}
	if uploadRes.Error != nil {
		logrus.WithError(uploadRes.Error).Error("failed to upload device keys")
		return true
	}

	return true
}
