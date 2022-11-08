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
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/storage"
	fedTypes "github.com/matrix-org/dendrite/federationapi/types"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

const maxRetries = 5
const retryDelay = time.Second

// OutputReceiptConsumer consumes events that originate in the clientapi.
type OutputPresenceConsumer struct {
	ctx                     context.Context
	jetstream               nats.JetStreamContext
	durable                 string
	db                      storage.Database
	queues                  *queue.OutgoingQueues
	isLocalServerName       func(gomatrixserverlib.ServerName) bool
	rsAPI                   roomserverAPI.FederationRoomserverAPI
	topic                   string
	outboundPresenceEnabled bool
}

// NewOutputPresenceConsumer creates a new OutputPresenceConsumer. Call Start() to begin consuming events.
func NewOutputPresenceConsumer(
	process *process.ProcessContext,
	cfg *config.FederationAPI,
	js nats.JetStreamContext,
	queues *queue.OutgoingQueues,
	store storage.Database,
	rsAPI roomserverAPI.FederationRoomserverAPI,
) *OutputPresenceConsumer {
	return &OutputPresenceConsumer{
		ctx:                     process.Context(),
		jetstream:               js,
		queues:                  queues,
		db:                      store,
		isLocalServerName:       cfg.Matrix.IsLocalServerName,
		durable:                 cfg.Matrix.JetStream.Durable("FederationAPIPresenceConsumer"),
		topic:                   cfg.Matrix.JetStream.Prefixed(jetstream.OutputPresenceEvent),
		outboundPresenceEnabled: cfg.Matrix.Presence.EnableOutbound,
		rsAPI:                   rsAPI,
	}
}

// Start consuming from the clientapi
func (t *OutputPresenceConsumer) Start() error {
	if !t.outboundPresenceEnabled {
		return nil
	}
	return jetstream.JetStreamConsumer(
		t.ctx, t.jetstream, t.topic, t.durable, 1, t.onMessage,
		nats.DeliverAll(), nats.ManualAck(), nats.HeadersOnly(),
	)
}

// onMessage is called in response to a message received on the presence
// events topic from the client api.
func (t *OutputPresenceConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	// only send presence events which originated from us
	userID := msg.Header.Get(jetstream.UserID)
	_, serverName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithError(err).WithField("user_id", userID).Error("failed to extract domain from receipt sender")
		return true
	}
	if !t.isLocalServerName(serverName) {
		return true
	}

	presence := msg.Header.Get("presence")

	ts, err := strconv.Atoi(msg.Header.Get("last_active_ts"))
	if err != nil {
		return true
	}

	var joined []gomatrixserverlib.ServerName
	// We're trying to get joined rooms for this user for 5 seconds (5 retries, 1s delay)
	// If we fail to get joined hosts, we discard the presence event.
	for i := 0; i < maxRetries; i++ {
		var queryRes roomserverAPI.QueryRoomsForUserResponse
		err = t.rsAPI.QueryRoomsForUser(t.ctx, &roomserverAPI.QueryRoomsForUserRequest{
			UserID:         userID,
			WantMembership: "join",
		}, &queryRes)
		if err != nil {
			log.WithError(err).Error("failed to calculate joined rooms for user")
			return true
		}

		joined, err = t.db.GetJoinedHostsForRooms(t.ctx, queryRes.RoomIDs, true)
		if err != nil {
			log.WithError(err).Error("failed to get joined hosts")
			return true
		}

		if len(joined) == 0 {
			time.Sleep(retryDelay)
			continue
		}
		break
	}
	// If we still have no joined hosts, discard the event.
	if len(joined) == 0 {
		return true
	}

	var statusMsg *string = nil
	if data, ok := msg.Header["status_msg"]; ok && len(data) > 0 {
		status := msg.Header.Get("status_msg")
		statusMsg = &status
	}

	p := types.PresenceInternal{LastActiveTS: gomatrixserverlib.Timestamp(ts)}

	content := fedTypes.Presence{
		Push: []fedTypes.PresenceContent{
			{
				CurrentlyActive: p.CurrentlyActive(),
				LastActiveAgo:   p.LastActiveAgo(),
				Presence:        presence,
				StatusMsg:       statusMsg,
				UserID:          userID,
			},
		},
	}

	edu := &gomatrixserverlib.EDU{
		Type:   gomatrixserverlib.MPresence,
		Origin: string(serverName),
	}
	if edu.Content, err = json.Marshal(content); err != nil {
		log.WithError(err).Error("failed to marshal EDU JSON")
		return true
	}

	log.Tracef("sending presence EDU to %d servers", len(joined))
	if err = t.queues.SendEDU(edu, serverName, joined); err != nil {
		log.WithError(err).Error("failed to send EDU")
		return false
	}

	return true
}
