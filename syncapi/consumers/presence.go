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

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/userapi/api"
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
	deviceAPI     api.UserDeviceAPI
	cfg           *config.SyncAPI
}

// NewPresenceConsumer creates a new PresenceConsumer.
// Call Start() to begin consuming events.
func NewPresenceConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	nats *nats.Conn,
	db storage.Database,
	notifier *notifier.Notifier,
	stream types.StreamProvider,
	deviceAPI api.UserDeviceAPI,
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
		deviceAPI:     deviceAPI,
		cfg:           cfg,
	}
}

// Start consuming typing events.
func (s *PresenceConsumer) Start() error {
	// Normal NATS subscription, used by Request/Reply
	_, err := s.nats.Subscribe(s.requestTopic, func(msg *nats.Msg) {
		userID := msg.Header.Get(jetstream.UserID)
		presence, err := s.db.GetPresence(context.Background(), userID)
		m := &nats.Msg{
			Header: nats.Header{},
		}
		if err != nil {
			m.Header.Set("error", err.Error())
			if err = msg.RespondMsg(m); err != nil {
				logrus.WithError(err).Error("Unable to respond to messages")
			}
			return
		}

		deviceRes := api.QueryDevicesResponse{}
		if err = s.deviceAPI.QueryDevices(s.ctx, &api.QueryDevicesRequest{UserID: userID}, &deviceRes); err != nil {
			m.Header.Set("error", err.Error())
			if err = msg.RespondMsg(m); err != nil {
				logrus.WithError(err).Error("Unable to respond to messages")
			}
			return
		}

		for i := range deviceRes.Devices {
			if int64(presence.LastActiveTS) < deviceRes.Devices[i].LastSeenTS {
				presence.LastActiveTS = gomatrixserverlib.Timestamp(deviceRes.Devices[i].LastSeenTS)
			}
		}

		m.Header.Set(jetstream.UserID, presence.UserID)
		m.Header.Set("presence", presence.ClientFields.Presence)
		m.Header.Set("status_msg", *presence.ClientFields.StatusMsg)
		m.Header.Set("last_active_ts", strconv.Itoa(int(presence.LastActiveTS)))

		if err = msg.RespondMsg(m); err != nil {
			logrus.WithError(err).Error("Unable to respond to messages")
			return
		}
	})
	if err != nil {
		return err
	}
	if !s.cfg.Matrix.Presence.EnableInbound && !s.cfg.Matrix.Presence.EnableOutbound {
		return nil
	}
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.presenceTopic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(), nats.HeadersOnly(),
	)
}

func (s *PresenceConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	userID := msg.Header.Get(jetstream.UserID)
	presence := msg.Header.Get("presence")
	timestamp := msg.Header.Get("last_active_ts")
	fromSync, _ := strconv.ParseBool(msg.Header.Get("from_sync"))

	logrus.Debugf("syncAPI received presence event: %+v", msg.Header)

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		return true
	}

	var statusMsg *string = nil
	if data, ok := msg.Header["status_msg"]; ok && len(data) > 0 {
		newMsg := msg.Header.Get("status_msg")
		statusMsg = &newMsg
	}
	// OK is already checked, so no need to do it again
	p, _ := types.PresenceFromString(presence)
	pos, err := s.db.UpdatePresence(ctx, userID, p, statusMsg, gomatrixserverlib.Timestamp(ts), fromSync)
	if err != nil {
		return true
	}

	s.stream.Advance(pos)
	s.notifier.OnNewPresence(types.StreamingToken{PresencePosition: pos}, userID)

	return true
}
