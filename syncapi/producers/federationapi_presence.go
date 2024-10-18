// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package producers

import (
	"strconv"
	"time"

	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
)

// FederationAPIPresenceProducer produces events for the federation API server to consume
type FederationAPIPresenceProducer struct {
	Topic     string
	JetStream nats.JetStreamContext
}

func (f *FederationAPIPresenceProducer) SendPresence(
	userID string, presence types.Presence, statusMsg *string,
) error {
	msg := nats.NewMsg(f.Topic)
	msg.Header.Set(jetstream.UserID, userID)
	msg.Header.Set("presence", presence.String())
	msg.Header.Set("from_sync", "true") // only update last_active_ts and presence
	msg.Header.Set("last_active_ts", strconv.Itoa(int(spec.AsTimestamp(time.Now()))))

	if statusMsg != nil {
		msg.Header.Set("status_msg", *statusMsg)
	}

	_, err := f.JetStream.PublishMsg(msg)
	return err
}
