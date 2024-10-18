// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"context"
	"strconv"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/syncapi/notifier"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/streams"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
	stream        streams.StreamProvider
	notifier      *notifier.Notifier
	deviceAPI     api.SyncUserAPI
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
	stream streams.StreamProvider,
	deviceAPI api.SyncUserAPI,
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
		presences, err := s.db.GetPresences(context.Background(), []string{userID})
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

		presence := &types.PresenceInternal{
			UserID: userID,
		}
		if len(presences) > 0 {
			presence = presences[0]
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
				presence.LastActiveTS = spec.Timestamp(deviceRes.Devices[i].LastSeenTS)
			}
		}

		m.Header.Set(jetstream.UserID, presence.UserID)
		m.Header.Set("presence", presence.ClientFields.Presence)
		if presence.ClientFields.StatusMsg != nil {
			m.Header.Set("status_msg", *presence.ClientFields.StatusMsg)
		}
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
		s.ctx, s.jetstream, s.presenceTopic, s.durable, 1, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(), nats.HeadersOnly(),
	)
}

func (s *PresenceConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	userID := msg.Header.Get(jetstream.UserID)
	presence := msg.Header.Get("presence")
	timestamp := msg.Header.Get("last_active_ts")
	fromSync, _ := strconv.ParseBool(msg.Header.Get("from_sync"))
	logrus.Tracef("syncAPI received presence event: %+v", msg.Header)

	if fromSync { // do not process local presence changes; we already did this synchronously.
		return true
	}

	ts, err := strconv.ParseUint(timestamp, 10, 64)
	if err != nil {
		return true
	}

	var statusMsg *string = nil
	if data, ok := msg.Header["status_msg"]; ok && len(data) > 0 {
		newMsg := msg.Header.Get("status_msg")
		statusMsg = &newMsg
	}
	// already checked, so no need to check error
	p, _ := types.PresenceFromString(presence)

	s.EmitPresence(ctx, userID, p, statusMsg, spec.Timestamp(ts), fromSync)
	return true
}

func (s *PresenceConsumer) EmitPresence(ctx context.Context, userID string, presence types.Presence, statusMsg *string, ts spec.Timestamp, fromSync bool) {
	pos, err := s.db.UpdatePresence(ctx, userID, presence, statusMsg, ts, fromSync)
	if err != nil {
		logrus.WithError(err).WithField("user", userID).WithField("presence", presence).Warn("failed to updated presence for user")
		return
	}
	s.stream.Advance(pos)
	s.notifier.OnNewPresence(types.StreamingToken{PresencePosition: pos}, userID)
}
