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

package producers

import (
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
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
	msg.Header.Set("last_active_ts", strconv.Itoa(int(gomatrixserverlib.AsTimestamp(time.Now()))))

	if statusMsg != nil {
		msg.Header.Set("status_msg", *statusMsg)
	}

	_, err := f.JetStream.PublishMsg(msg)
	return err
}
