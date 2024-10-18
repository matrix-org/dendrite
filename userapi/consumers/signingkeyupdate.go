// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/userapi/api"
)

// SigningKeyUpdateConsumer consumes signing key updates that came in over federation.
type SigningKeyUpdateConsumer struct {
	ctx               context.Context
	jetstream         nats.JetStreamContext
	durable           string
	topic             string
	userAPI           api.UploadDeviceKeysAPI
	cfg               *config.UserAPI
	isLocalServerName func(spec.ServerName) bool
}

// NewSigningKeyUpdateConsumer creates a new SigningKeyUpdateConsumer. Call Start() to begin consuming from key servers.
func NewSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	userAPI api.UploadDeviceKeysAPI,
) *SigningKeyUpdateConsumer {
	return &SigningKeyUpdateConsumer{
		ctx:               process.Context(),
		jetstream:         js,
		durable:           cfg.Matrix.JetStream.Prefixed("KeyServerSigningKeyConsumer"),
		topic:             cfg.Matrix.JetStream.Prefixed(jetstream.InputSigningKeyUpdate),
		userAPI:           userAPI,
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
	var updatePayload api.CrossSigningKeyUpdate
	if err := json.Unmarshal(msg.Data, &updatePayload); err != nil {
		logrus.WithError(err).Errorf("Failed to read from signing key update input topic")
		return true
	}
	origin := spec.ServerName(msg.Header.Get("origin"))
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

	keys := fclient.CrossSigningKeys{}
	if updatePayload.MasterKey != nil {
		keys.MasterKey = *updatePayload.MasterKey
	}
	if updatePayload.SelfSigningKey != nil {
		keys.SelfSigningKey = *updatePayload.SelfSigningKey
	}
	uploadReq := &api.PerformUploadDeviceKeysRequest{
		CrossSigningKeys: keys,
		UserID:           updatePayload.UserID,
	}
	uploadRes := &api.PerformUploadDeviceKeysResponse{}
	t.userAPI.PerformUploadDeviceKeys(ctx, uploadReq, uploadRes)
	if uploadRes.Error != nil {
		logrus.WithError(uploadRes.Error).Error("failed to upload device keys")
		return true
	}

	return true
}
