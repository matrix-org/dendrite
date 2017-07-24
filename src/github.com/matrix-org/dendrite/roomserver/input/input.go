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
	"encoding/json"
	"fmt"
	"sync/atomic"

	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/util"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// RoomserverInputAPI implements api.RoomserverInputAPI
type RoomserverInputAPI struct {
	DB       RoomEventDatabase
	Producer sarama.SyncProducer
	// The kafkaesque topic to output new room events to.
	// This is the name used in kafka to identify the stream to write events to.
	OutputRoomEventTopic string
	// If non-nil then the API will stop processing messages after this
	// many messages and will shutdown. Malformed messages are not in the count.
	StopProcessingAfter *int64
	// If not-nil then the API will call this to shutdown the server.
	// If this is nil then the API will continue to process messsages even
	// though StopProcessingAfter has been reached.
	ShutdownCallback func(reason string)
	// How many messages the consumer has processed.
	processed int64
}

// WriteOutputEvents implements OutputRoomEventWriter
func (r *RoomserverInputAPI) WriteOutputEvents(roomID string, updates []api.OutputEvent) error {
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

// InputRoomEvents implements api.RoomserverInputAPI
func (r *RoomserverInputAPI) InputRoomEvents(
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) error {
	for i := range request.InputRoomEvents {
		if err := processRoomEvent(r.DB, r, request.InputRoomEvents[i]); err != nil {
			return err
		}
		// Update the number of processed messages using atomic addition because it is accessed from multiple goroutines.
		processed := atomic.AddInt64(&r.processed, 1)
		// Check if we should stop processing.
		// Note that since we have multiple goroutines it's quite likely that we'll overshoot by a few messages.
		// If we try to stop processing after M message and we have N goroutines then we will process somewhere
		// between M and (N + M) messages because the N goroutines could all try to process what they think will be the
		// last message. We could be more careful here but this is good enough for getting rough benchmarks.
		if r.StopProcessingAfter != nil && processed >= int64(*r.StopProcessingAfter) {
			if r.ShutdownCallback != nil {
				r.ShutdownCallback(fmt.Sprintf("Stopping processing after %d messages", r.processed))
			}
		}
	}
	return nil
}

// SetupHTTP adds the RoomserverInputAPI handlers to the http.ServeMux.
func (r *RoomserverInputAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(api.RoomserverInputRoomEventsPath,
		common.MakeAPI("inputRoomEvents", func(req *http.Request) util.JSONResponse {
			var request api.InputRoomEventsRequest
			var response api.InputRoomEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(400, err.Error())
			}
			if err := r.InputRoomEvents(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
}
