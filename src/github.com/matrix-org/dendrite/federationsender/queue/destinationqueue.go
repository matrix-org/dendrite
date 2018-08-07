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

package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// destinationQueue is a queue of events for a single destination.
// It is responsible for sending the events to the destination and
// ensures that only one request is in flight to a given destination
// at a time.
type destinationQueue struct {
	client      *gomatrixserverlib.FederationClient
	origin      gomatrixserverlib.ServerName
	destination gomatrixserverlib.ServerName
	// The running mutex protects running, sentCounter, lastTransactionIDs and
	// pendingEvents, pendingEDUs.
	runningMutex       sync.Mutex
	running            bool
	sentCounter        int
	lastTransactionIDs []gomatrixserverlib.TransactionID
	pendingEvents      []*gomatrixserverlib.Event
	pendingEDUs        []*gomatrixserverlib.EDU
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(ev *gomatrixserverlib.Event) {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()
	oq.pendingEvents = append(oq.pendingEvents, ev)
	if !oq.running {
		oq.running = true
		go oq.backgroundSend()
	}
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending event to that destination.
func (oq *destinationQueue) sendEDU(ev *gomatrixserverlib.EDU) {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()
	oq.pendingEDUs = append(oq.pendingEDUs, ev)
	if !oq.running {
		oq.running = true
		go oq.backgroundSend()
	}
}

func (oq *destinationQueue) backgroundSend() {
	for {
		t := oq.next()
		if t == nil {
			// If the queue is empty then stop processing for this destination.
			// TODO: Remove this destination from the queue map.
			return
		}

		// TODO: handle retries.
		// TODO: blacklist uncooperative servers.

		_, err := oq.client.SendTransaction(context.TODO(), *t)
		if err != nil {
			log.WithFields(log.Fields{
				"destination": oq.destination,
				log.ErrorKey:  err,
			}).Info("problem sending transaction")
		}
	}
}

// next creates a new transaction from the pending event queue
// and flushes the queue.
// Returns nil if the queue was empty.
func (oq *destinationQueue) next() *gomatrixserverlib.Transaction {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()

	if len(oq.pendingEvents) == 0 && len(oq.pendingEDUs) == 0 {
		oq.running = false
		return nil
	}

	var t gomatrixserverlib.Transaction
	now := gomatrixserverlib.AsTimestamp(time.Now())
	t.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, oq.sentCounter))
	t.Origin = oq.origin
	t.Destination = oq.destination
	t.OriginServerTS = now
	t.PreviousIDs = oq.lastTransactionIDs
	if t.PreviousIDs == nil {
		t.PreviousIDs = []gomatrixserverlib.TransactionID{}
	}

	oq.lastTransactionIDs = []gomatrixserverlib.TransactionID{t.TransactionID}

	for _, pdu := range oq.pendingEvents {
		t.PDUs = append(t.PDUs, *pdu)
	}
	oq.pendingEvents = nil
	oq.sentCounter += len(t.PDUs)

	for _, edu := range oq.pendingEDUs {
		t.EDUs = append(t.EDUs, *edu)
	}
	oq.pendingEDUs = nil

	return &t
}
