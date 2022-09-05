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
	"encoding/json"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	fedapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/keyserver/producers"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	ctx       context.Context
	cfg       *config.KeyServer
	jetstream nats.JetStreamContext
	durable   string
	db        storage.Database
	topic     string
	producer  *producers.KeyChange
	fedClient fedapi.KeyserverFederationAPI
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.KeyServer,
	js nats.JetStreamContext,
	store storage.Database,
	keyChangeProducer *producers.KeyChange,
	fedClient fedapi.KeyserverFederationAPI,

) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:       process.Context(),
		cfg:       cfg,
		jetstream: js,
		db:        store,
		durable:   cfg.Matrix.JetStream.Durable("KeyserverAPIRoomServerConsumer"),
		topic:     cfg.Matrix.JetStream.Prefixed(jetstream.OutputRoomEvent),
		producer:  keyChangeProducer,
		fedClient: fedClient,
	}
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, 1,
		s.onMessage, nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called when the federation server receives a new event from the room server output log.
// It is unsafe to call this with messages for the same room in multiple gorountines
// because updates it will likely fail with a types.EventIDMismatchError when it
// realises that it cannot update the room state using the deltas.
func (s *OutputRoomEventConsumer) onMessage(ctx context.Context, msgs []*nats.Msg) bool {
	msg := msgs[0] // Guaranteed to exist if onMessage is called
	// Parse out the event JSON
	var output api.OutputEvent
	if err := json.Unmarshal(msg.Data, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return true
	}

	switch output.Type {
	case api.OutputTypeNewRoomEvent:
		log.Debugf("XXX: got room event")
		ev := output.NewRoomEvent.Event
		// Only handle m.room.member events
		if ev.Type() != gomatrixserverlib.MRoomMember || ev.StateKey() == nil {
			log.Debugf("XXX: ignoring, not a membership event")
			return true
		}

		_, domain, err := gomatrixserverlib.SplitID('@', *ev.StateKey())
		if err != nil {
			log.WithError(err).Errorf("unable to split statekey")
			return true
		}

		// ignore membership changes from other servers
		if domain != s.cfg.Matrix.ServerName {
			log.Debugf("XXX: ignoring event from different server")
			return true
		}

		curMembership, err := ev.Membership()
		// we only care about join events
		if err != nil {
			log.WithError(err).Errorf("unable to parse membership event")
			return true
		}
		if curMembership != gomatrixserverlib.Join {
			log.Debugf("XXX: ignoring event, current membership is %s", curMembership)
			return true
		}
		prevMembership := gjson.GetBytes(ev.Unsigned(), "prev_content.membership").Str

		switch prevMembership {
		case gomatrixserverlib.Invite:
			fallthrough
		case gomatrixserverlib.Knock:
			fallthrough
		case gomatrixserverlib.Leave:
			fallthrough
		case "": // there's no prev membership, inform remote
			devMessages, err := s.db.DeviceKeysForUser(ctx, *ev.StateKey(), []string{}, true)
			if err != nil {
				log.WithError(err).Errorf("unable to get device keys for user")
				return false // we want to retry this
			}
			log.Debugf("XXX: producing key changes")

			return s.producer.ProduceKeyChanges(devMessages) == nil
		default:
			log.Debugf("ignoring unexpected prev membership: '%s'", prevMembership)
			return true
		}
	default: // ignore all other events
	}
	return true
}
