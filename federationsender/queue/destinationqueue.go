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
	"time"

	"github.com/matrix-org/dendrite/federationsender/producers"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrix"
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
	rsProducer         *producers.RoomserverProducer           // roomserver producer
	client             *gomatrixserverlib.FederationClient     // federation client
	origin             gomatrixserverlib.ServerName            // origin of requests
	destination        gomatrixserverlib.ServerName            // destination of requests
	running            atomic.Bool                             // is the queue worker running?
	backingOff         atomic.Bool                             // true if we're backing off
	statistics         *types.ServerStatistics                 // statistics about this remote server
	incomingPDUs       chan *gomatrixserverlib.HeaderedEvent   // PDUs to send
	incomingEDUs       chan *gomatrixserverlib.EDU             // EDUs to send
	incomingInvites    chan *gomatrixserverlib.InviteV2Request // invites to send
	lastTransactionIDs []gomatrixserverlib.TransactionID       // last transaction ID
	pendingPDUs        []*gomatrixserverlib.HeaderedEvent      // owned by backgroundSend
	pendingEDUs        []*gomatrixserverlib.EDU                // owned by backgroundSend
	pendingInvites     []*gomatrixserverlib.InviteV2Request    // owned by backgroundSend
	retryServerCh      chan bool                               // interrupts backoff
}

// retry will clear the blacklist state and attempt to send built up events to the server,
// resetting and interrupting any backoff timers.
func (oq *destinationQueue) retry() {
	// TODO: We don't send all events in the case where the server has been blacklisted as we
	// drop events instead then. This means we will send the oldest N events (chan size, currently 128)
	// and then skip ahead a lot which feels non-ideal but equally we can't persist thousands of events
	// in-memory to maybe-send it one day. Ideally we would just shove these pending events in a database
	// so we can send a lot of events.
	oq.statistics.Success()
	// if we were backing off, swap to not backing off and interrupt the select.
	// We need to use an atomic bool here to prevent multiple calls to retry() blocking on the channel
	// as it is unbuffered.
	if oq.backingOff.CAS(true, false) {
		oq.retryServerCh <- true
	}
	if !oq.running.Load() {
		log.Infof("Restarting queue for %s", oq.destination)
		go oq.backgroundSend()
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
	if !oq.running.Load() {
		go oq.backgroundSend()
	}
	oq.incomingPDUs <- ev
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEDU(ev *gomatrixserverlib.EDU) {
	if oq.statistics.Blacklisted() {
		// If the destination is blacklisted then drop the event.
		return
	}
	if !oq.running.Load() {
		go oq.backgroundSend()
	}
	oq.incomingEDUs <- ev
}

// sendInvite adds the invite event to the pending queue for the
// destination. If the queue is empty then it starts a background
// goroutine to start sending events to that destination.
func (oq *destinationQueue) sendInvite(ev *gomatrixserverlib.InviteV2Request) {
	if oq.statistics.Blacklisted() {
		// If the destination is blacklisted then drop the event.
		return
	}
	if !oq.running.Load() {
		go oq.backgroundSend()
	}
	oq.incomingInvites <- ev
}

// backgroundSend is the worker goroutine for sending events.
// nolint:gocyclo
func (oq *destinationQueue) backgroundSend() {
	// Check if a worker is already running, and if it isn't, then
	// mark it as started.
	if !oq.running.CAS(false, true) {
		return
	}
	defer oq.running.Store(false)

	for {
		// Wait either for incoming events, or until we hit an
		// idle timeout.
		select {
		case pdu := <-oq.incomingPDUs:
			// Ordering of PDUs is important so we add them to the end
			// of the queue and they will all be added to transactions
			// in order.
			oq.pendingPDUs = append(oq.pendingPDUs, pdu)
			// If there are any more things waiting in the channel queue
			// then read them. This is safe because we guarantee only
			// having one goroutine per destination queue, so the channel
			// isn't being consumed anywhere else.
			for len(oq.incomingPDUs) > 0 {
				oq.pendingPDUs = append(oq.pendingPDUs, <-oq.incomingPDUs)
			}
		case edu := <-oq.incomingEDUs:
			// Likewise for EDUs, although we should probably not try
			// too hard with some EDUs (like typing notifications) after
			// a certain amount of time has passed.
			// TODO: think about EDU expiry some more
			oq.pendingEDUs = append(oq.pendingEDUs, edu)
			// If there are any more things waiting in the channel queue
			// then read them. This is safe because we guarantee only
			// having one goroutine per destination queue, so the channel
			// isn't being consumed anywhere else.
			for len(oq.incomingEDUs) > 0 {
				oq.pendingEDUs = append(oq.pendingEDUs, <-oq.incomingEDUs)
			}
		case invite := <-oq.incomingInvites:
			// There's no strict ordering requirement for invites like
			// there is for transactions, so we put the invite onto the
			// front of the queue. This means that if an invite that is
			// stuck failing already, that it won't block our new invite
			// from being sent.
			oq.pendingInvites = append(
				[]*gomatrixserverlib.InviteV2Request{invite},
				oq.pendingInvites...,
			)
			// If there are any more things waiting in the channel queue
			// then read them. This is safe because we guarantee only
			// having one goroutine per destination queue, so the channel
			// isn't being consumed anywhere else.
			for len(oq.incomingInvites) > 0 {
				oq.pendingInvites = append(oq.pendingInvites, <-oq.incomingInvites)
			}
		case <-time.After(time.Second * 30):
			// The worker is idle so stop the goroutine. It'll
			// get restarted automatically the next time we
			// get an event.
			return
		}

		// If we are backing off this server then wait for the
		// backoff duration to complete first, or until explicitly
		// told to retry.
		if backoff, duration := oq.statistics.BackoffDuration(); backoff {
			oq.backingOff.Store(true)
			select {
			case <-time.After(duration):
			case <-oq.retryServerCh:
			}
			oq.backingOff.Store(false)
		}

		// How many things do we have waiting?
		numPDUs := len(oq.pendingPDUs)
		numEDUs := len(oq.pendingEDUs)
		numInvites := len(oq.pendingInvites)

		// If we have pending PDUs or EDUs then construct a transaction.
		if numPDUs > 0 || numEDUs > 0 {
			// Try sending the next transaction and see what happens.
			transaction, terr := oq.nextTransaction(oq.pendingPDUs, oq.pendingEDUs, oq.statistics.SuccessCount())
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
			}
		}

		// Try sending the next invite and see what happens.
		if numInvites > 0 {
			sent, ierr := oq.nextInvites(oq.pendingInvites)
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
				// Reallocate so that the underlying array can be GC'd, as
				// opposed to growing forever.
				oq.pendingInvites = append(
					[]*gomatrixserverlib.InviteV2Request{},
					oq.pendingInvites[sent:]...,
				)
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

	// TODO: we should check for 500-ish fails vs 400-ish here,
	// since we shouldn't queue things indefinitely in response
	// to a 400-ish error
	_, err := oq.client.SendTransaction(context.TODO(), t)
	switch e := err.(type) {
	case nil:
		// No error was returned so the transaction looks to have
		// been successfully sent.
		return true, nil
	case gomatrix.HTTPError:
		// We received a HTTP error back. In this instance we only
		// should report an error if
		if e.Code >= 400 && e.Code <= 499 {
			// We tried but the remote side has sent back a client error.
			// It's no use retrying because it will happen again.
			return true, nil
		}
		// Otherwise, report that we failed to send the transaction
		// and we will retry again.
		return false, err
	default:
		log.WithFields(log.Fields{
			"destination": oq.destination,
			log.ErrorKey:  err,
		}).Info("problem sending transaction")
		return false, err
	}
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
		switch e := err.(type) {
		case nil:
			done++
		case gomatrix.HTTPError:
			log.WithFields(log.Fields{
				"event_id":    ev.EventID(),
				"state_key":   ev.StateKey(),
				"destination": oq.destination,
				"status_code": e.Code,
			}).WithError(err).Error("failed to send invite due to HTTP error")
			// Check whether we should do something about the error or
			// just accept it as unavoidable.
			if e.Code >= 400 && e.Code <= 499 {
				// We tried but the remote side has sent back a client error.
				// It's no use retrying because it will happen again.
				done++
				continue
			}
			return done, err
		default:
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
	}

	return done, nil
}
