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
	"time"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

type Inputer struct {
	DB                   storage.Database
	Producer             sarama.SyncProducer
	ServerName           gomatrixserverlib.ServerName
	ACLs                 *acls.ServerACLs
	OutputRoomEventTopic string

	workers sync.Map // room ID -> *inputWorker
}

type inputTask struct {
	ctx   context.Context
	event *api.InputRoomEvent
	wg    *sync.WaitGroup
	err   error // written back by worker, only safe to read when all tasks are done
}

type inputWorker struct {
	r       *Inputer
	running atomic.Bool
	input   chan *inputTask
}

func (w *inputWorker) start() {
	if !w.running.CAS(false, true) {
		return
	}
	defer w.running.Store(false)
	for {
		select {
		case task := <-w.input:
			_, task.err = w.r.processRoomEvent(task.ctx, task.event)
			task.wg.Done()
		case <-time.After(time.Second * 5):
			return
		}
	}
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
				defer r.ACLs.OnServerACLUpdate(&ev)
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
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
	// Create a wait group. Each task that we dispatch will call Done on
	// this wait group so that we know when all of our events have been
	// processed.
	wg := &sync.WaitGroup{}
	wg.Add(len(request.InputRoomEvents))
	tasks := make([]*inputTask, len(request.InputRoomEvents))

	for i, e := range request.InputRoomEvents {
		// Work out if we are running per-room workers or if we're just doing
		// it on a global basis (e.g. SQLite).
		roomID := "global"
		if r.DB.SupportsConcurrentRoomInputs() {
			roomID = e.Event.RoomID()
		}

		// Look up the worker, or create it if it doesn't exist. This channel
		// is buffered to reduce the chance that we'll be blocked by another
		// room - the channel will be quite small as it's just pointer types.
		w, _ := r.workers.LoadOrStore(roomID, &inputWorker{
			r:     r,
			input: make(chan *inputTask, 10),
		})
		worker := w.(*inputWorker)

		// Create a task. This contains the input event and a reference to
		// the wait group, so that the worker can notify us when this specific
		// task has been finished.
		tasks[i] = &inputTask{
			ctx:   ctx,
			event: &request.InputRoomEvents[i],
			wg:    wg,
		}

		// Send the task to the worker.
		go worker.start()
		worker.input <- tasks[i]
	}

	// Wait for all of the workers to return results about our tasks.
	wg.Wait()

	// If any of the tasks returned an error, we should probably report
	// that back to the caller.
	for _, task := range tasks {
		if task.err != nil {
			response.ErrMsg = task.err.Error()
			_, rejected := task.err.(*gomatrixserverlib.NotAllowed)
			response.NotAllowed = rejected
			return
		}
	}
}
