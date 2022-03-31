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
	"strconv"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// OutputTypingEventConsumer consumes events that originated in the EDU server.
type PresenceConsumer struct {
	ctx           context.Context
	jetstream     nats.JetStreamContext
	nats          *nats.Conn
	durable       string
	requestTopic  string
	presenceTopic string
	db            storage.Database
	stream        types.StreamProvider
	notifier      *notifier.Notifier
	rsAPI         api.RoomserverInternalAPI
}

// NewOutputTypingEventConsumer creates a new OutputTypingEventConsumer.
// Call Start() to begin consuming from the EDU server.
func NewPresenceConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	nats *nats.Conn,
	db storage.Database,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
	rsAPI api.RoomserverInternalAPI,
) *PresenceConsumer {
	return &PresenceConsumer{
		ctx:           process.Context(),
		nats:          nats,
		jetstream:     js,
		durable:       cfg.Matrix.JetStream.Durable("SyncAPIPresenceConsumer"),
		presenceTopic: cfg.Matrix.JetStream.Prefixed(jetstream.OutputPresenceEvent),
		requestTopic:  cfg.Matrix.JetStream.Prefixed(jetstream.RequestPresence),
		db:            db,
		notifier:      notifier,
		stream:        stream,
		rsAPI:         rsAPI,
	}
}

// Start consuming typing events.
func (s *PresenceConsumer) Start() error {
	// Normal NATS subscription, used by Request/Reply
	_, err := s.nats.Subscribe(s.requestTopic, func(msg *nats.Msg) {
		presence, err := s.db.GetPresence(context.Background(), msg.Header.Get(jetstream.UserID))
		m := &nats.Msg{
			Header: nats.Header{},
		}
		if err != nil {
			m.Header.Set("error", err.Error())
			if err = msg.RespondMsg(m); err != nil {
				return
			}
			return
		}

		m.Header.Set(jetstream.UserID, presence.UserID)
		m.Header.Set("presence", presence.ClientFields.Presence)
		m.Header.Set("status_msg", *presence.ClientFields.StatusMsg)
		m.Header.Set("last_active_ts", strconv.Itoa(int(presence.LastActiveTS)))

		if err = msg.RespondMsg(m); err != nil {
			return
		}
	})
	if err != nil {
		return err
	}
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.presenceTopic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(), nats.HeadersOnly(),
	)
}

func (s *PresenceConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	userID := msg.Header.Get(jetstream.UserID)
	presence := msg.Header.Get("presence")
	statusMsg := msg.Header.Get("status_msg")
	timestamp := msg.Header.Get("last_active_ts")
	fromSync, _ := strconv.ParseBool(msg.Header.Get("from_sync"))
	nilStatusMsg, _ := strconv.ParseBool(msg.Header.Get("status_msg_nil"))

	logrus.Debugf("syncAPI received presence event: %+v", msg.Header)

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		return true
	}

	newStatusMsg := &statusMsg
	if nilStatusMsg {
		newStatusMsg = nil
	}

	pos, err := s.db.UpdatePresence(ctx, userID, presence, newStatusMsg, gomatrixserverlib.Timestamp(ts), fromSync)
	if err != nil {
		return true
	}

	s.stream.Advance(pos)
	s.notifier.OnNewPresence(types.StreamingToken{PresencePosition: pos}, userID)

	return true
}
