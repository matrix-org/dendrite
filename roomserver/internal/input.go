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
package internal

import (
	"context"
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/roomserver/api"

	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
)

// SetFederationSenderInputAPI passes in a federation sender input API reference
// so that we can avoid the chicken-and-egg problem of both the roomserver input API
// and the federation sender input API being interdependent.
func (r *RoomserverInternalAPI) SetFederationSenderAPI(fsAPI fsAPI.FederationSenderInternalAPI) {
	r.fsAPI = fsAPI
}

// WriteOutputEvents implements OutputRoomEventWriter
func (r *RoomserverInternalAPI) WriteOutputEvents(roomID string, updates []api.OutputEvent) error {
	messages := make([]*sarama.ProducerMessage, len(updates))
	for i := range updates {
		value, err := json.Marshal(updates[i])
		if err != nil {
			return err
		}
		messages[i] = &sarama.ProducerMessage{
			Topic: r.OutputRoomEventTopic,
			Key:   sarama.StringEncoder(roomID),
			Value: sarama.ByteEncoder(value),
		}
	}
	return r.Producer.SendMessages(messages)
}

// InputRoomEvents implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) (err error) {
	// We lock as processRoomEvent can only be called once at a time
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for i := range request.InputInviteEvents {
		if event, e := processInviteEvent(ctx, r.DB, r, request.InputInviteEvents[i]); e != nil {
			return e
		} else {
			request.InputRoomEvents = append(request.InputRoomEvents, api.InputRoomEvent{
				Kind:         api.KindNew,
				Event:        *event,
				AuthEventIDs: event.AuthEventIDs(),
			})
		}
	}
	for i := range request.InputRoomEvents {
		if response.EventID, err = processRoomEvent(ctx, r.DB, r, request.InputRoomEvents[i]); err != nil {
			return err
		}
	}
	return nil
}
