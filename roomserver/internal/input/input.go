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

// Package input contains the code processes new room events
package input

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type Inputer struct {
	DB                   storage.Database
	Producer             sarama.SyncProducer
	ServerName           gomatrixserverlib.ServerName
	ACLs                 *acls.ServerACLs
	OutputRoomEventTopic string

	latestEventsMutexes sync.Map // room ID -> sync.Mutex
}

// WriteOutputEvents implements OutputRoomEventWriter
func (r *Inputer) WriteOutputEvents(roomID string, updates []api.OutputEvent) error {
	messages := make([]*sarama.ProducerMessage, len(updates))
	for i := range updates {
		value, err := json.Marshal(updates[i])
		if err != nil {
			return err
		}
		logger := log.WithFields(log.Fields{
			"room_id": roomID,
			"type":    updates[i].Type,
		})
		if updates[i].NewRoomEvent != nil {
			logger = logger.WithFields(log.Fields{
				"event_type":     updates[i].NewRoomEvent.Event.Type(),
				"event_id":       updates[i].NewRoomEvent.Event.EventID(),
				"adds_state":     len(updates[i].NewRoomEvent.AddsStateEventIDs),
				"removes_state":  len(updates[i].NewRoomEvent.RemovesStateEventIDs),
				"send_as_server": updates[i].NewRoomEvent.SendAsServer,
				"sender":         updates[i].NewRoomEvent.Event.Sender(),
			})
			if updates[i].NewRoomEvent.Event.Type() == "m.room.server_acl" && updates[i].NewRoomEvent.Event.StateKeyEquals("") {
				ev := updates[i].NewRoomEvent.Event.Unwrap()
				defer r.ACLs.OnServerACLUpdate(ev)
			}
		}
		logger.Infof("Producing to topic '%s'", r.OutputRoomEventTopic)
		messages[i] = &sarama.ProducerMessage{
			Topic: r.OutputRoomEventTopic,
			Key:   sarama.StringEncoder(roomID),
			Value: sarama.ByteEncoder(value),
		}
	}
	errs := r.Producer.SendMessages(messages)
	if errs != nil {
		for _, err := range errs.(sarama.ProducerErrors) {
			log.WithError(err).WithField("message_bytes", err.Msg.Value.Length()).Error("Write to kafka failed")
		}
	}
	return errs
}

// InputRoomEvents implements api.RoomserverInternalAPI
func (r *Inputer) InputRoomEvents(
	_ context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
	for i := range request.InputRoomEvents {
		hooks.Run(hooks.KindNewEventReceived, &request.InputRoomEvents[i])
		if _, err := r.processRoomEvent(context.Background(), &request.InputRoomEvents[i]); err == nil {
			hooks.Run(hooks.KindNewEventPersisted, &request.InputRoomEvents[i])
		} else {
			response.ErrMsg = err.Error()
			_, rejected := err.(*gomatrixserverlib.NotAllowed)
			response.NotAllowed = rejected
		}
	}
}
