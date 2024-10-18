// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"context"
	"encoding/json"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/federationapi/queue"
	"github.com/element-hq/dendrite/federationapi/storage"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	syncTypes "github.com/element-hq/dendrite/syncapi/types"
)

// OutputSendToDeviceConsumer consumes events that originate in the clientapi.
type OutputSendToDeviceConsumer struct {
	ctx               context.Context
	jetstream         nats.JetStreamContext
	durable           string
	db                storage.Database
	queues            *queue.OutgoingQueues
	isLocalServerName func(spec.ServerName) bool
	topic             string
}

// NewOutputSendToDeviceConsumer creates a new OutputSendToDeviceConsumer. Call Start() to begin consuming send-to-device events.
func NewOutputSendToDeviceConsumer(
	process *process.ProcessContext,
	cfg *config.FederationAPI,
	js nats.JetStreamContext,
	queues *queue.OutgoingQueues,
	store storage.Database,
) *OutputSendToDeviceConsumer {
	return &OutputSendToDeviceConsumer{
		ctx:               process.Context(),
		jetstream:         js,
		queues:            queues,
		db:                store,
		isLocalServerName: cfg.Matrix.IsLocalServerName,
		durable:           cfg.Matrix.JetStream.Durable("FederationAPIESendToDeviceConsumer"),
		topic:             cfg.Matrix.JetStream.Prefixed(jetstream.OutputSendToDeviceEvent),
	}
}

// Start consuming from the client api
func (t *OutputSendToDeviceConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.topic, t.durable, 1,
		t.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called in response to a message received on the
// send-to-device events topic from the client api.
func (t *OutputSendToDeviceConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	// only send send-to-device events which originated from us
	sender := msg.Header.Get("sender")
	_, originServerName, err := gomatrixserverlib.SplitID('@', sender)
	if err != nil {
		sentry.CaptureException(err)
		log.WithError(err).WithField("user_id", sender).Error("Failed to extract domain from send-to-device sender")
		return true
	}
	if !t.isLocalServerName(originServerName) {
		return true
	}
	// Extract the send-to-device event from msg.
	var ote syncTypes.OutputSendToDeviceEvent
	if err = json.Unmarshal(msg.Data, &ote); err != nil {
		sentry.CaptureException(err)
		log.WithError(err).Errorf("output log: message parse failed (expected send-to-device)")
		return true
	}

	_, destServerName, err := gomatrixserverlib.SplitID('@', ote.UserID)
	if err != nil {
		sentry.CaptureException(err)
		log.WithError(err).WithField("user_id", ote.UserID).Error("Failed to extract domain from send-to-device destination")
		return true
	}

	// The SyncAPI is already handling sendToDevice for the local server
	if t.isLocalServerName(destServerName) {
		return true
	}

	// Pack the EDU and marshal it
	edu := &gomatrixserverlib.EDU{
		Type:   spec.MDirectToDevice,
		Origin: string(originServerName),
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
		sentry.CaptureException(err)
		log.WithError(err).Error("failed to marshal EDU JSON")
		return true
	}

	log.Debugf("Sending send-to-device message into %q destination queue", destServerName)
	if err := t.queues.SendEDU(edu, originServerName, []spec.ServerName{destServerName}); err != nil {
		log.WithError(err).Error("failed to send EDU")
		return false
	}

	return true
}
