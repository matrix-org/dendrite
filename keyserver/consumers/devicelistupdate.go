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

	"github.com/matrix-org/dendrite/keyserver/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// DeviceListUpdateConsumer consumes device list updates that came in over federation.
type DeviceListUpdateConsumer struct {
	ctx       context.Context
	jetstream nats.JetStreamContext
	durable   string
	topic     string
	updater   *internal.DeviceListUpdater
}

// NewDeviceListUpdateConsumer creates a new DeviceListConsumer. Call Start() to begin consuming from key servers.
func NewDeviceListUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.KeyServer,
	js nats.JetStreamContext,
	updater *internal.DeviceListUpdater,
) *DeviceListUpdateConsumer {
	return &DeviceListUpdateConsumer{
		ctx:       process.Context(),
		jetstream: js,
		durable:   cfg.Matrix.JetStream.Prefixed("KeyServerInputDeviceListConsumer"),
		topic:     cfg.Matrix.JetStream.Prefixed(jetstream.InputDeviceListUpdate),
		updater:   updater,
	}
}

// Start consuming from key servers
func (t *DeviceListUpdateConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.topic, t.durable, t.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called in response to a message received on the
// key change events topic from the key server.
func (t *DeviceListUpdateConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	var m gomatrixserverlib.DeviceListUpdateEvent
	if err := json.Unmarshal(msg.Data, &m); err != nil {
		logrus.WithError(err).Errorf("Failed to read from device list update input topic")
		return true
	}
	err := t.updater.Update(ctx, m)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"user_id":   m.UserID,
			"device_id": m.DeviceID,
			"stream_id": m.StreamID,
			"prev_id":   m.PrevID,
		}).WithError(err).Errorf("Failed to update device list")
		return false
	}
	return true
}
