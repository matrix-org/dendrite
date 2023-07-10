// Copyright 2017 Vector Creations Ltd
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
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage/mrd"
	"github.com/matrix-org/dendrite/syncapi/streams"
	"github.com/matrix-org/dendrite/syncapi/types"
)

// OutputMultiRoomDataConsumer consumes events that originated in the client API server.
type OutputMultiRoomDataConsumer struct {
	ctx       context.Context
	jetstream nats.JetStreamContext
	durable   string
	topic     string
	db        *mrd.Queries
	stream    streams.StreamProvider
	notifier  *notifier.Notifier
}

// NewOutputMultiRoomDataConsumer creates a new OutputMultiRoomDataConsumer consumer. Call Start() to begin consuming from room servers.
func NewOutputMultiRoomDataConsumer(
	process *process.ProcessContext,
	cfg *config.SyncAPI,
	js nats.JetStreamContext,
	q *mrd.Queries,
	notifier *notifier.Notifier,
	stream streams.StreamProvider,
) *OutputMultiRoomDataConsumer {
	return &OutputMultiRoomDataConsumer{
		ctx:       process.Context(),
		jetstream: js,
		topic:     cfg.Matrix.JetStream.Prefixed(jetstream.OutputMultiRoomCast),
		durable:   cfg.Matrix.JetStream.Durable("SyncAPIMultiRoomDataConsumer"),
		db:        q,
		notifier:  notifier,
		stream:    stream,
	}
}

func (s *OutputMultiRoomDataConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, 1,
		s.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

func (s *OutputMultiRoomDataConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0]
	userID := msg.Header.Get(jetstream.UserID)
	dataType := msg.Header.Get("type")

	log.WithFields(log.Fields{
		"type":    dataType,
		"user_id": userID,
	}).Debug("Received multiroom data from client API server")

	pos, err := s.db.InsertMultiRoomData(ctx, mrd.InsertMultiRoomDataParams{
		UserID: userID,
		Type:   dataType,
		Data:   msg.Data,
	})
	if err != nil {
		sentry.CaptureException(err)
		log.WithFields(log.Fields{
			"type":    dataType,
			"user_id": userID,
		}).WithError(err).Errorf("could not insert multi room data")
		return false
	}

	rooms, err := s.db.SelectMultiRoomVisibilityRooms(ctx, mrd.SelectMultiRoomVisibilityRoomsParams{
		UserID:   userID,
		ExpireTs: time.Now().UnixMilli(),
	})
	if err != nil {
		sentry.CaptureException(err)
		log.WithFields(log.Fields{
			"type":    dataType,
			"user_id": userID,
		}).WithError(err).Errorf("failed to select multi room visibility")
		return false
	}

	s.stream.Advance(types.StreamPosition(pos))
	s.notifier.OnNewMultiRoomData(types.StreamingToken{MultiRoomDataPosition: types.StreamPosition(pos)}, rooms)

	return true
}
