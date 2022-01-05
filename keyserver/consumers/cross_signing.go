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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"

	"github.com/Shopify/sarama"
)

type OutputCrossSigningKeyUpdateConsumer struct {
	eduServerConsumer *internal.ContinualConsumer
	keyDB             storage.Database
	keyAPI            api.KeyInternalAPI
	serverName        string
}

func NewOutputCrossSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	keyDB storage.Database,
	keyAPI api.KeyInternalAPI,
) *OutputCrossSigningKeyUpdateConsumer {
	// The keyserver both produces and consumes on the TopicOutputKeyChangeEvent
	// topic. We will only produce events where the UserID matches our server name,
	// and we will only consume events where the UserID does NOT match our server
	// name (because the update came from a remote server).
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "keyserver/keyserver",
		Topic:          cfg.Global.JetStream.TopicFor(jetstream.OutputKeyChangeEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: keyDB,
	}
	s := &OutputCrossSigningKeyUpdateConsumer{
		eduServerConsumer: &consumer,
		keyDB:             keyDB,
		keyAPI:            keyAPI,
		serverName:        string(cfg.Global.ServerName),
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

func (s *OutputCrossSigningKeyUpdateConsumer) Start() error {
	return s.eduServerConsumer.Start()
}

// onMessage is called in response to a message received on the
// key change events topic from the key server.
func (t *OutputCrossSigningKeyUpdateConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var m api.DeviceMessage
	if err := json.Unmarshal(msg.Value, &m); err != nil {
		logrus.WithError(err).Errorf("failed to read device message from key change topic")
		return nil
	}
	if m.OutputCrossSigningKeyUpdate == nil {
		// This probably shouldn't happen but stops us from panicking if we come
		// across an update that doesn't satisfy either types.
		return nil
	}
	switch m.Type {
	case api.TypeCrossSigningUpdate:
		return t.onCrossSigningMessage(m)
	default:
		return nil
	}
}

func (s *OutputCrossSigningKeyUpdateConsumer) onCrossSigningMessage(m api.DeviceMessage) error {
	output := m.CrossSigningKeyUpdate
	_, host, err := gomatrixserverlib.SplitID('@', output.UserID)
	if err != nil {
		logrus.WithError(err).Errorf("eduserver output log: user ID parse failure")
		return nil
	}
	if host == gomatrixserverlib.ServerName(s.serverName) {
		// Ignore any messages that contain information about our own users, as
		// they already originated from this server.
		return nil
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
	return uploadRes.Error
}
