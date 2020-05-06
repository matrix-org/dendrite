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
	"math"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationsender/producers"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

const (
	// How many times should we tolerate consecutive failures before we
	// just blacklist the host altogether? Bear in mind that the backoff
	// is exponential, so the max time here to attempt is 2**failures.
	FailuresUntilBlacklist = 16 // 16 equates to roughly 18 hours.
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
	blacklisted        atomic.Bool                          // is the remote side dead?
	backoffUntil       atomic.Value                         // time.Time to wait until before sending requests
	idleCounter        atomic.Uint32                        // how many ticks have we done nothing?
	failCounter        atomic.Uint32                        // how many times have we failed?
	sentCounter        atomic.Uint32                        // how many times have we succeeded?
	wakeup             chan bool                            // wakes up a sleeping worker
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
		oq.wakeup <- true
	}
}

// Backoff marks a failure and works out when to back off until. It
// returns true if the worker should give up altogether because of
// too many consecutive failures.
func (oq *destinationQueue) backoff() bool {
	// Increase the fail counter.
	failCounter := oq.failCounter.Load()
	failCounter++
	oq.failCounter.Store(failCounter)

	// Check that we haven't failed more times than is acceptable.
	if failCounter < FailuresUntilBlacklist {
		// We're still under the threshold so work out the exponential
		// backoff based on how many times we have failed already. The
		// worker goroutine will wait until this time before processing
		// anything from the queue.
		backoffSeconds := time.Second * time.Duration(math.Exp2(float64(failCounter)))
		oq.backoffUntil.Store(
			time.Now().Add(backoffSeconds),
		)
		logrus.WithField("server_name", oq.destination).Infof("Increasing backoff to %s", backoffSeconds)
		return false // Don't give up yet.
	} else {
		// We've exceeded the maximum amount of times we're willing
		// to back off, which is probably in the region of hours by
		// now. Just give up - clear the queues and reset the queue
		// back to its default state.
		oq.blacklisted.Store(true)
		oq.runningMutex.Lock()
		oq.pendingPDUs = nil
		oq.pendingEDUs = nil
		oq.pendingInvites = nil
		oq.runningMutex.Unlock()
		logrus.WithField("server_name", oq.destination).Infof("Blacklisting server due to %d consecutive errors", failCounter)
		return true // Give up.
	}
}

func (oq *destinationQueue) success() {
	// Reset the idle and fail counters.
	oq.idleCounter.Store(0)
	oq.failCounter.Store(0)

	// Increase the sent counter.
	sentCounter := oq.failCounter.Load()
	oq.sentCounter.Store(sentCounter + 1)
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(ev *gomatrixserverlib.HeaderedEvent) {
	if oq.blacklisted.Load() {
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
	if oq.blacklisted.Load() {
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
	if oq.blacklisted.Load() {
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
		// Wait for our backoff timer.
		backoffUntil := time.Now()
		if b, ok := oq.backoffUntil.Load().(time.Time); ok {
			backoffUntil = b
		}

		// If we have a backoff period then sit and wait for it.
		if backoffUntil.After(time.Now()) {
			<-time.After(time.Until(backoffUntil))
		}

		// Retrieve any waiting things.
		oq.runningMutex.RLock()
		pendingPDUs, pendingEDUs := oq.pendingPDUs, oq.pendingEDUs
		pendingInvites := oq.pendingInvites
		idleCounter, sentCounter := oq.idleCounter.Load(), oq.sentCounter.Load()
		oq.runningMutex.RUnlock()

		// If we have pending PDUs or EDUs then construct a transaction.
		if len(pendingPDUs) > 0 || len(pendingEDUs) > 0 {
			// Try sending the next transaction and see what happens.
			transaction, terr := oq.nextTransaction(pendingPDUs, pendingEDUs, sentCounter)
			if terr != nil {
				// We failed to send the transaction.
				if giveUp := oq.backoff(); giveUp {
					// It's been suggested that we should give up because
					// the backoff has exceeded a maximum allowable value.
					return
				}
				continue
			}

			// If we successfully sent the transaction then clear out
			// the pending events and EDUs.
			if transaction {
				oq.runningMutex.Lock()
				// Reallocate so that the underlying arrays can be GC'd, as
				// opposed to growing forever.
				oq.pendingPDUs = append(
					[]*gomatrixserverlib.HeaderedEvent{},
					oq.pendingPDUs[len(pendingPDUs):]...,
				)
				oq.pendingEDUs = append(
					[]*gomatrixserverlib.EDU{},
					oq.pendingEDUs[len(pendingEDUs):]...,
				)
				oq.runningMutex.Unlock()
			}
		}

		// Try sending the next invite and see what happens.
		if len(pendingInvites) > 0 {
			invites, ierr := oq.nextInvites(pendingInvites)
			if ierr != nil {
				// We failed to send the transaction so increase the
				// backoff and give it another go shortly.
				oq.backoffUntil.Store(time.Until(backoffUntil) * 2)
				continue
			}

			// If we successfully sent the invites then clear out
			// the pending invites.
			if invites {
				oq.runningMutex.Lock()
				// Reallocate so that the underlying array can be GC'd, as
				// opposed to growing forever.
				oq.pendingInvites = append(
					[]*gomatrixserverlib.InviteV2Request{},
					oq.pendingInvites[len(pendingInvites):]...,
				)
				oq.runningMutex.Unlock()
			}
		}

		// If everything was fine at this point then we can update
		// the counters for the transaction IDs.
		oq.success()

		// Wait either for a few seconds, or until a new event is
		// available.
		select {
		case <-oq.wakeup:
		case <-time.After(time.Second * 5):
			// If this worker has been idle for a while then stop
			// running it, otherwise the goroutine will just tick
			// endlessly. It'll get automatically restarted when
			// a new event needs to be sent.
			if idleCounter >= 5 {
				return
			}
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
) (bool, error) {
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
			return false, err
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
			return false, err
		}
	}

	return true, nil
}
