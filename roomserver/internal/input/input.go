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
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

type Inputer struct {
	DB                   storage.Database
	Producer             sarama.SyncProducer
	ServerName           gomatrixserverlib.ServerName
	OutputRoomEventTopic string

	workers sync.Map // room ID -> *inputWorker
}

type inputTask struct {
	event   api.InputRoomEvent
	wg      *sync.WaitGroup
	eventID string // written back by worker
	err     error  // written back by worker
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

	logrus.Warn("STARTING WORKER")
	defer logrus.Warn("SHUTTING DOWN WORKER")

	for {
		select {
		case task := <-w.input:
			logrus.Warn("WORKER DOING TASK")
			task.eventID, task.err = w.r.processRoomEvent(context.TODO(), task.event)
			logrus.Warn("WORKER FINISHING TASK")
			task.wg.Done()
			logrus.Warn("WORKER FINISHED TASK")
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
		}
		logger.Infof("Producing to topic '%s'", r.OutputRoomEventTopic)
		messages[i] = &sarama.ProducerMessage{
			Topic: r.OutputRoomEventTopic,
			Key:   sarama.StringEncoder(roomID),
			Value: sarama.ByteEncoder(value),
		}
	}
	return r.Producer.SendMessages(messages)
}

// InputRoomEvents implements api.RoomserverInternalAPI
func (r *Inputer) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) (err error) {
	wg := &sync.WaitGroup{}
	wg.Add(len(request.InputRoomEvents))
	tasks := make([]*inputTask, len(request.InputRoomEvents))
	logrus.Warnf("Received %d input events", len(tasks))

	for i, e := range request.InputRoomEvents {
		// Work out if we are running per-room workers or if we're just doing
		// it on a global basis (e.g. SQLite).
		roomID := "global"
		if r.DB.SupportsConcurrentRoomInputs() {
			roomID = e.Event.RoomID()
		}

		// Look up the worker, or create it if it doesn't exist.
		w, _ := r.workers.LoadOrStore(roomID, &inputWorker{
			r:     r,
			input: make(chan *inputTask),
		})
		worker := w.(*inputWorker)

		// Create a task. This contains the input event and a reference to
		// the wait group, so that the worker can notify us when this specific
		// task has been finished.
		tasks[i] = &inputTask{
			event: e,
			wg:    wg,
		}

		// Send the task to the worker.
		go func(task *inputTask) { worker.input <- task }(tasks[i])
		go worker.start()
	}

	logrus.Warnf("Waiting for %d task(s)", len(tasks))
	wg.Wait()
	logrus.Warnf("Tasks finished")

	for _, task := range tasks {
		if task.err != nil {
			logrus.Warnf("Error: %w", task.err)
		} else {
			logrus.Warnf("Event ID: %s", task.eventID)
		}
	}

	return nil
}
