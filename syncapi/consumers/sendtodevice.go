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

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// OutputSendToDeviceEventConsumer consumes events that originated in the EDU server.
type OutputSendToDeviceEventConsumer struct {
	ctx        context.Context
	jetstream  nats.JetStreamContext
	durable    string
	topic      string
	db         storage.Database
	serverName gomatrixserverlib.ServerName // our server name
	stream     types.StreamProvider
	notifier   *notifier.Notifier
}

// NewOutputSendToDeviceEventConsumer creates a new OutputSendToDeviceEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewOutputSendToDeviceEventConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	store storage.Database,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
) *OutputSendToDeviceEventConsumer {
	return &OutputSendToDeviceEventConsumer{
		ctx:        process.Context(),
		jetstream:  js,
		topic:      cfg.Matrix.JetStream.Prefixed(jetstream.OutputSendToDeviceEvent),
		durable:    cfg.Matrix.JetStream.Durable("SyncAPISendToDeviceConsumer"),
		db:         store,
		serverName: cfg.Matrix.ServerName,
		notifier:   notifier,
		stream:     stream,
	}
}

// Start consuming send-to-device events.
func (s *OutputSendToDeviceEventConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

func (s *OutputSendToDeviceEventConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	userID := msg.Header.Get(jetstream.UserID)
	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		sentry.CaptureException(err)
		return true
	}
	if domain != s.serverName {
		return true
	}

	var output types.OutputSendToDeviceEvent
	if err = json.Unmarshal(msg.Data, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("output log: message parse failure")
		sentry.CaptureException(err)
		return true
	}

	util.GetLogger(context.TODO()).WithFields(log.Fields{
		"sender":     output.Sender,
		"user_id":    output.UserID,
		"device_id":  output.DeviceID,
		"event_type": output.Type,
	}).Debugf("sync API received send-to-device event from the clientapi/federationsender")

	streamPos, err := s.db.StoreNewSendForDeviceMessage(
		s.ctx, output.UserID, output.DeviceID, output.SendToDeviceEvent,
	)
	if err != nil {
		sentry.CaptureException(err)
		log.WithError(err).Errorf("failed to store send-to-device message")
		return false
	}

	s.stream.Advance(streamPos)
	s.notifier.OnNewSendToDevice(
		output.UserID,
		[]string{output.DeviceID},
		types.StreamingToken{SendToDevicePosition: streamPos},
	)

	return true
}
