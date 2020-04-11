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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

// destinationQueue is a queue of events for a single destination.
// It is responsible for sending the events to the destination and
// ensures that only one request is in flight to a given destination
// at a time.
type destinationQueue struct {
	client      *gomatrixserverlib.FederationClient
	origin      gomatrixserverlib.ServerName
	destination gomatrixserverlib.ServerName
	running     atomic.Bool
	// The running mutex protects sentCounter, lastTransactionIDs and
	// pendingEvents, pendingEDUs.
	runningMutex       sync.Mutex
	sentCounter        int
	lastTransactionIDs []gomatrixserverlib.TransactionID
	pendingEvents      []*gomatrixserverlib.HeaderedEvent
	pendingEDUs        []*gomatrixserverlib.EDU
	pendingInvites     []*gomatrixserverlib.InviteV2Request
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(ev *gomatrixserverlib.HeaderedEvent) {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()
	oq.pendingEvents = append(oq.pendingEvents, ev)
	if !oq.running.Load() {
		go oq.backgroundSend()
	}
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEDU(e *gomatrixserverlib.EDU) {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()
	oq.pendingEDUs = append(oq.pendingEDUs, e)
	if !oq.running.Load() {
		go oq.backgroundSend()
	}
}

// sendInvite adds the invite event to the pending queue for the
// destination. If the queue is empty then it starts a background
// goroutine to start sending events to that destination.
func (oq *destinationQueue) sendInvite(ev *gomatrixserverlib.InviteV2Request) {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()
	oq.pendingInvites = append(oq.pendingInvites, ev)
	if !oq.running.Load() {
		go oq.backgroundSend()
	}
}

// backgroundSend is the worker goroutine for sending events.
func (oq *destinationQueue) backgroundSend() {
	oq.running.Store(true)
	defer oq.running.Store(false)

	for {
		transaction, invites := oq.nextTransaction(), oq.nextInvites()
		if !transaction && !invites {
			// If the queue is empty then stop processing for this destination.
			// TODO: Remove this destination from the queue map.
			return
		}

		// TODO: handle retries.
		// TODO: blacklist uncooperative servers.
	}
}

// nextTransaction creates a new transaction from the pending event
// queue and sends it. Returns true if a transaction was sent or
// false otherwise.
func (oq *destinationQueue) nextTransaction() bool {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()

	if len(oq.pendingEvents) == 0 && len(oq.pendingEDUs) == 0 {
		return false
	}

	t := gomatrixserverlib.Transaction{
		PDUs: []json.RawMessage{},
		EDUs: []gomatrixserverlib.EDU{},
	}
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
		// Append the JSON of the event, since this is a json.RawMessage type in the
		// gomatrixserverlib.Transaction struct
		t.PDUs = append(t.PDUs, (*pdu).JSON())
	}
	oq.pendingEvents = nil
	oq.sentCounter += len(t.PDUs)

	for _, edu := range oq.pendingEDUs {
		t.EDUs = append(t.EDUs, *edu)
	}
	oq.pendingEDUs = nil
	oq.sentCounter += len(t.EDUs)

	util.GetLogger(context.TODO()).Infof("Sending transaction %q containing %d PDUs, %d EDUs", t.TransactionID, len(t.PDUs), len(t.EDUs))

	_, err := oq.client.SendTransaction(context.TODO(), t)
	if err != nil {
		log.WithFields(log.Fields{
			"destination": oq.destination,
			log.ErrorKey:  err,
		}).Info("problem sending transaction")
	}

	return true
}

// nextInvite takes pending invite events from the queue and sends
// them. Returns true if a transaction was sent or false otherwise.
func (oq *destinationQueue) nextInvites() bool {
	oq.runningMutex.Lock()
	defer oq.runningMutex.Unlock()

	if len(oq.pendingInvites) == 0 {
		return false
	}

	for _, inviteReq := range oq.pendingInvites {
		ev := inviteReq.Event()

		if _, err := oq.client.SendInviteV2(
			context.TODO(),
			oq.destination,
			*inviteReq,
		); err != nil {
			log.WithFields(log.Fields{
				"event_id":    ev.EventID(),
				"state_key":   ev.StateKey(),
				"destination": oq.destination,
			}).WithError(err).Error("failed to send invite")
		}
	}

	oq.pendingInvites = nil

	return true
}
