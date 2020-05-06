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

	"github.com/matrix-org/dendrite/federationsender/producers"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

// destinationQueue is a queue of events for a single destination.
// It is responsible for sending the events to the destination and
// ensures that only one request is in flight to a given destination
// at a time.
type destinationQueue struct {
	rsProducer         *producers.RoomserverProducer        // roomserver producer
	client             *gomatrixserverlib.FederationClient  // federation client
	origin             gomatrixserverlib.ServerName         // origin of requests
	destination        gomatrixserverlib.ServerName         // destination of requests
	running            atomic.Bool                          // is the queue worker running?
	wakeup             chan bool                            // wakes up a sleeping worker
	statistics         *types.ServerStatistics              // statistics about this remote server
	idleCounter        atomic.Uint32                        // how many ticks have we done nothing?
	runningMutex       sync.RWMutex                         // protects the below
	lastTransactionIDs []gomatrixserverlib.TransactionID    // protected by runningMutex
	pendingPDUs        []*gomatrixserverlib.HeaderedEvent   // protected by runningMutex
	pendingEDUs        []*gomatrixserverlib.EDU             // protected by runningMutex
	pendingInvites     []*gomatrixserverlib.InviteV2Request // protected by runningMutex
}

// Start the destination queue if it needs to be started, or
// otherwise signal to it that it should wake up from sleep.
func (oq *destinationQueue) wake() {
	if !oq.running.Load() {
		go oq.backgroundSend()
	} else {
		select {
		case oq.wakeup <- true:
		default:
		}
	}
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(ev *gomatrixserverlib.HeaderedEvent) {
	if oq.statistics.Blacklisted() {
		// If the destination is blacklisted then drop the event.
		return
	}
	oq.runningMutex.Lock()
	oq.pendingPDUs = append(oq.pendingPDUs, ev)
	oq.runningMutex.Unlock()
	oq.wake()
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEDU(ev *gomatrixserverlib.EDU) {
	if oq.statistics.Blacklisted() {
		// If the destination is blacklisted then drop the event.
		return
	}
	oq.runningMutex.Lock()
	oq.pendingEDUs = append(oq.pendingEDUs, ev)
	oq.runningMutex.Unlock()
	oq.wake()
}

// sendInvite adds the invite event to the pending queue for the
// destination. If the queue is empty then it starts a background
// goroutine to start sending events to that destination.
func (oq *destinationQueue) sendInvite(ev *gomatrixserverlib.InviteV2Request) {
	if oq.statistics.Blacklisted() {
		// If the destination is blacklisted then drop the event.
		return
	}
	oq.runningMutex.Lock()
	oq.pendingInvites = append(oq.pendingInvites, ev)
	oq.runningMutex.Unlock()
	oq.wake()
}

// backgroundSend is the worker goroutine for sending events.
// nolint:gocyclo
func (oq *destinationQueue) backgroundSend() {
	// Mark the worker as running for its lifetime.
	oq.wakeup = make(chan bool)
	oq.running.Store(true)
	defer oq.running.Store(false)
	defer close(oq.wakeup)

	for {
		// If we are backing off this server then wait for the
		// backoff duration to complete first.
		if backoff, duration := oq.statistics.BackoffDuration(); backoff {
			<-time.After(duration)
		}

		// Retrieve any waiting things.
		oq.runningMutex.RLock()
		pendingPDUs, numPDUs := oq.pendingPDUs, len(oq.pendingPDUs)
		pendingEDUs, numEDUs := oq.pendingEDUs, len(oq.pendingEDUs)
		pendingInvites := oq.pendingInvites
		idleCounter, sentCounter := oq.idleCounter.Load(), oq.statistics.SuccessCount()
		oq.runningMutex.RUnlock()

		// If this worker has been idle for a while then stop
		// running it, otherwise the goroutine will just tick
		// endlessly. It'll get automatically restarted when
		// a new event needs to be sent.
		if idleCounter >= 5 {
			return
		}
		if len(pendingInvites) == 0 && len(pendingPDUs) == 0 && len(pendingEDUs) == 0 {
			oq.idleCounter.Add(1)
		}

		// If we have pending PDUs or EDUs then construct a transaction.
		if len(pendingPDUs) > 0 || len(pendingEDUs) > 0 {
			// Try sending the next transaction and see what happens.
			transaction, terr := oq.nextTransaction(pendingPDUs, pendingEDUs, sentCounter)
			if terr != nil {
				// We failed to send the transaction.
				if giveUp := oq.statistics.Failure(); giveUp {
					// It's been suggested that we should give up because
					// the backoff has exceeded a maximum allowable value.
					return
				}
			} else if transaction {
				// If we successfully sent the transaction then clear out
				// the pending events and EDUs.
				oq.statistics.Success()
				oq.runningMutex.Lock()
				// Reallocate so that the underlying arrays can be GC'd, as
				// opposed to growing forever.
				for i := 0; i < numPDUs; i++ {
					oq.pendingPDUs[i] = nil
				}
				for i := 0; i < numEDUs; i++ {
					oq.pendingEDUs[i] = nil
				}
				oq.pendingPDUs = append(
					[]*gomatrixserverlib.HeaderedEvent{},
					oq.pendingPDUs[numPDUs:]...,
				)
				oq.pendingEDUs = append(
					[]*gomatrixserverlib.EDU{},
					oq.pendingEDUs[numEDUs:]...,
				)
				oq.runningMutex.Unlock()
			}
		}

		// Try sending the next invite and see what happens.
		if len(pendingInvites) > 0 {
			sent, ierr := oq.nextInvites(pendingInvites)
			if ierr != nil {
				// We failed to send the transaction so increase the
				// backoff and give it another go shortly.
				if giveUp := oq.statistics.Failure(); giveUp {
					// It's been suggested that we should give up because
					// the backoff has exceeded a maximum allowable value.
					return
				}
			} else if sent > 0 {
				// If we successfully sent the invites then clear out
				// the pending invites.
				oq.statistics.Success()
				oq.runningMutex.Lock()
				// Reallocate so that the underlying array can be GC'd, as
				// opposed to growing forever.
				oq.pendingInvites = append(
					[]*gomatrixserverlib.InviteV2Request{},
					oq.pendingInvites[sent:]...,
				)
				oq.runningMutex.Unlock()
			}
		}

		// Wait either for a few seconds, or until a new event is
		// available.
		select {
		case <-oq.wakeup:
		case <-time.After(time.Second * 5):
		}
	}
}

// nextTransaction creates a new transaction from the pending event
// queue and sends it. Returns true if a transaction was sent or
// false otherwise.
func (oq *destinationQueue) nextTransaction(
	pendingPDUs []*gomatrixserverlib.HeaderedEvent,
	pendingEDUs []*gomatrixserverlib.EDU,
	sentCounter uint32,
) (bool, error) {
	t := gomatrixserverlib.Transaction{
		PDUs: []json.RawMessage{},
		EDUs: []gomatrixserverlib.EDU{},
	}
	now := gomatrixserverlib.AsTimestamp(time.Now())
	t.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, sentCounter))
	t.Origin = oq.origin
	t.Destination = oq.destination
	t.OriginServerTS = now
	t.PreviousIDs = oq.lastTransactionIDs
	if t.PreviousIDs == nil {
		t.PreviousIDs = []gomatrixserverlib.TransactionID{}
	}

	oq.lastTransactionIDs = []gomatrixserverlib.TransactionID{t.TransactionID}

	for _, pdu := range pendingPDUs {
		// Append the JSON of the event, since this is a json.RawMessage type in the
		// gomatrixserverlib.Transaction struct
		t.PDUs = append(t.PDUs, (*pdu).JSON())
	}

	for _, edu := range pendingEDUs {
		t.EDUs = append(t.EDUs, *edu)
	}

	logrus.WithField("server_name", oq.destination).Infof("Sending transaction %q containing %d PDUs, %d EDUs", t.TransactionID, len(t.PDUs), len(t.EDUs))

	_, err := oq.client.SendTransaction(context.TODO(), t)
	if err != nil {
		log.WithFields(log.Fields{
			"destination": oq.destination,
			log.ErrorKey:  err,
		}).Info("problem sending transaction")
		return false, err
	}

	return true, nil
}

// nextInvite takes pending invite events from the queue and sends
// them. Returns true if a transaction was sent or false otherwise.
func (oq *destinationQueue) nextInvites(
	pendingInvites []*gomatrixserverlib.InviteV2Request,
) (int, error) {
	done := 0
	for _, inviteReq := range pendingInvites {
		ev, roomVersion := inviteReq.Event(), inviteReq.RoomVersion()

		log.WithFields(log.Fields{
			"event_id":     ev.EventID(),
			"room_version": roomVersion,
			"destination":  oq.destination,
		}).Info("sending invite")

		inviteRes, err := oq.client.SendInviteV2(
			context.TODO(),
			oq.destination,
			*inviteReq,
		)
		if err != nil {
			log.WithFields(log.Fields{
				"event_id":    ev.EventID(),
				"state_key":   ev.StateKey(),
				"destination": oq.destination,
			}).WithError(err).Error("failed to send invite")
			return done, err
		}

		if _, err = oq.rsProducer.SendInviteResponse(
			context.TODO(),
			inviteRes,
			roomVersion,
		); err != nil {
			log.WithFields(log.Fields{
				"event_id":    ev.EventID(),
				"state_key":   ev.StateKey(),
				"destination": oq.destination,
			}).WithError(err).Error("failed to return signed invite to roomserver")
			return done, err
		}

		done++
	}

	return done, nil
}
