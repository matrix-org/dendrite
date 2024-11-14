// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package producers

import (
	"github.com/nats-io/nats.go"
)

// AppserviceEventProducer produces events for the appservice API to consume
type AppserviceEventProducer struct {
	Topic     string
	JetStream nats.JetStreamContext
}

func (a *AppserviceEventProducer) ProduceRoomEvents(
	msg *nats.Msg,
) error {
	msg.Subject = a.Topic
	_, err := a.JetStream.PublishMsg(msg)
	return err
}
