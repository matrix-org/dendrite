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

	"github.com/matrix-org/dendrite/federationsender/statistics"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

const (
	maxPDUsPerTransaction = 50
	maxEDUsPerTransaction = 50
	queueIdleTimeout      = time.Second * 30
)

// destinationQueue is a queue of events for a single destination.
// It is responsible for sending the events to the destination and
// ensures that only one request is in flight to a given destination
// at a time.
type destinationQueue struct {
	db                 storage.Database
	signing            *SigningInfo
	rsAPI              api.RoomserverInternalAPI
	client             *gomatrixserverlib.FederationClient     // federation client
	origin             gomatrixserverlib.ServerName            // origin of requests
	destination        gomatrixserverlib.ServerName            // destination of requests
	running            atomic.Bool                             // is the queue worker running?
	backingOff         atomic.Bool                             // true if we're backing off
	statistics         *statistics.ServerStatistics            // statistics about this remote server
	incomingInvites    chan *gomatrixserverlib.InviteV2Request // invites to send
	transactionIDMutex sync.Mutex                              // protects transactionID
	transactionID      gomatrixserverlib.TransactionID         // last transaction ID
	transactionCount   atomic.Int32                            // how many events in this transaction so far
	pendingInvites     []*gomatrixserverlib.InviteV2Request    // owned by backgroundSend
	notifyPDUs         chan bool                               // interrupts idle wait for PDUs
	notifyEDUs         chan bool                               // interrupts idle wait for EDUs
	interruptBackoff   chan bool                               // interrupts backoff
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(receipt *shared.Receipt) {
	// Create a transaction ID. We'll either do this if we don't have
	// one made up yet, or if we've exceeded the number of maximum
	// events allowed in a single tranaction. We'll reset the counter
	// when we do.
	oq.transactionIDMutex.Lock()
	if oq.transactionID == "" || oq.transactionCount.Load() >= maxPDUsPerTransaction {
		now := gomatrixserverlib.AsTimestamp(time.Now())
		oq.transactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, oq.statistics.SuccessCount()))
		oq.transactionCount.Store(0)
	}
	oq.transactionIDMutex.Unlock()
	// Create a database entry that associates the given PDU NID with
	// this destination queue. We'll then be able to retrieve the PDU
	// later.
	if err := oq.db.AssociatePDUWithDestination(
		context.TODO(),
		oq.transactionID, // the current transaction ID
		oq.destination,   // the destination server name
		receipt,          // NIDs from federationsender_queue_json table
	); err != nil {
		log.WithError(err).Errorf("failed to associate PDU receipt %q with destination %q", receipt.String(), oq.destination)
		return
	}
	// We've successfully added a PDU to the transaction so increase
	// the counter.
	oq.transactionCount.Add(1)
	// Check if the destination is blacklisted. If it isn't then wake
	// up the queue.
	if !oq.statistics.Blacklisted() {
		// Wake up the queue if it's asleep.
		oq.wakeQueueIfNeeded()
		// If we're blocking on waiting PDUs then tell the queue that we
		// have work to do.
		select {
		case oq.notifyPDUs <- true:
		default:
		}
	}
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEDU(receipt *shared.Receipt) {
	// Create a database entry that associates the given PDU NID with
	// this destination queue. We'll then be able to retrieve the PDU
	// later.
	if err := oq.db.AssociateEDUWithDestination(
		context.TODO(),
		oq.destination, // the destination server name
		receipt,        // NIDs from federationsender_queue_json table
	); err != nil {
		log.WithError(err).Errorf("failed to associate EDU receipt %q with destination %q", receipt.String(), oq.destination)
		return
	}
	// We've successfully added an EDU to the transaction so increase
	// the counter.
	oq.transactionCount.Add(1)
	// Check if the destination is blacklisted. If it isn't then wake
	// up the queue.
	if !oq.statistics.Blacklisted() {
		// Wake up the queue if it's asleep.
		oq.wakeQueueIfNeeded()
		// If we're blocking on waiting EDUs then tell the queue that we
		// have work to do.
		select {
		case oq.notifyEDUs <- true:
		default:
		}
	}
}

// sendInvite adds the invite event to the pending queue for the
// destination. If the queue is empty then it starts a background
// goroutine to start sending events to that destination.
func (oq *destinationQueue) sendInvite(ev *gomatrixserverlib.InviteV2Request) {
	if oq.statistics.Blacklisted() {
		// If the destination is blacklisted then drop the event.
		return
	}
	oq.wakeQueueIfNeeded()
	oq.incomingInvites <- ev
}

// wakeQueueIfNeeded will wake up the destination queue if it is
// not already running. If it is running but it is backing off
// then we will interrupt the backoff, causing any federation
// requests to retry.
func (oq *destinationQueue) wakeQueueIfNeeded() {
	// If we are backing off then interrupt the backoff.
	if oq.backingOff.CAS(true, false) {
		oq.interruptBackoff <- true
	}
	// If we aren't running then wake up the queue.
	if !oq.running.Load() {
		// Start the queue.
		go oq.backgroundSend()
	}
}

// waitForPDUs returns a channel for pending PDUs, which will be
// used in backgroundSend select. It returns a closed channel if
// there is something pending right now, or an open channel if
// we're waiting for something.
func (oq *destinationQueue) waitForPDUs() chan bool {
	pendingPDUs, err := oq.db.GetPendingPDUCount(context.TODO(), oq.destination)
	if err != nil {
		log.WithError(err).Errorf("Failed to get pending PDU count on queue %q", oq.destination)
	}
	// If there are PDUs pending right now then we'll return a closed
	// channel. This will mean that the backgroundSend will not block.
	if pendingPDUs > 0 {
		ch := make(chan bool, 1)
		close(ch)
		return ch
	}
	// If there are no PDUs pending right now then instead we'll return
	// the notify channel, so that backgroundSend can pick up normal
	// notifications from sendEvent.
	return oq.notifyPDUs
}

// waitForEDUs returns a channel for pending EDUs, which will be
// used in backgroundSend select. It returns a closed channel if
// there is something pending right now, or an open channel if
// we're waiting for something.
func (oq *destinationQueue) waitForEDUs() chan bool {
	pendingEDUs, err := oq.db.GetPendingEDUCount(context.TODO(), oq.destination)
	if err != nil {
		log.WithError(err).Errorf("Failed to get pending EDU count on queue %q", oq.destination)
	}
	// If there are EDUs pending right now then we'll return a closed
	// channel. This will mean that the backgroundSend will not block.
	if pendingEDUs > 0 {
		ch := make(chan bool, 1)
		close(ch)
		return ch
	}
	// If there are no EDUs pending right now then instead we'll return
	// the notify channel, so that backgroundSend can pick up normal
	// notifications from sendEvent.
	return oq.notifyEDUs
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
		pendingPDUs, pendingEDUs := false, false

		// If we have nothing to do then wait either for incoming events, or
		// until we hit an idle timeout.
		select {
		case <-oq.waitForPDUs():
			// We were woken up because there are new PDUs waiting in the
			// database.
			pendingPDUs = true
		case <-oq.waitForEDUs():
			// We were woken up because there are new PDUs waiting in the
			// database.
			pendingEDUs = true
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
		case <-time.After(queueIdleTimeout):
			// The worker is idle so stop the goroutine. It'll get
			// restarted automatically the next time we have an event to
			// send.
			log.Infof("Queue %q has been idle for %s, going to sleep", oq.destination, queueIdleTimeout)
			return
		}

		// If we are backing off this server then wait for the
		// backoff duration to complete first, or until explicitly
		// told to retry.
		if backoff, duration := oq.statistics.BackoffDuration(); backoff {
			log.WithField("duration", duration).Infof("Backing off %s", oq.destination)
			oq.backingOff.Store(true)
			select {
			case <-time.After(duration):
			case <-oq.interruptBackoff:
				log.Infof("Interrupting backoff for %q", oq.destination)
			}
			oq.backingOff.Store(false)
		}

		// If we have pending PDUs or EDUs then construct a transaction.
		if pendingPDUs || pendingEDUs {
			// Try sending the next transaction and see what happens.
			transaction, terr := oq.nextTransaction()
			if terr != nil {
				// We failed to send the transaction.
				if giveUp := oq.statistics.Failure(); giveUp {
					// It's been suggested that we should give up because the backoff
					// has exceeded a maximum allowable value. Clean up the in-memory
					// buffers at this point. The PDU clean-up is already on a defer.
					oq.cleanPendingInvites()
					log.Infof("Blacklisting %q due to errors", oq.destination)
					return
				} else {
					// We haven't been told to give up terminally yet but we still have
					// PDUs waiting to be sent. By sending a message into the wake chan,
					// the next loop iteration will try processing these PDUs again,
					// subject to the backoff.
					select {
					case oq.notifyPDUs <- true:
					default:
					}
				}
			} else if transaction {
				// If we successfully sent the transaction then clear out
				// the pending events and EDUs, and wipe our transaction ID.
				oq.statistics.Success()
			}
		}

		// Try sending the next invite and see what happens.
		if len(oq.pendingInvites) > 0 {
			sent, ierr := oq.nextInvites(oq.pendingInvites)
			if ierr != nil {
				// We failed to send the transaction so increase the
				// backoff and give it another go shortly.
				if giveUp := oq.statistics.Failure(); giveUp {
					// It's been suggested that we should give up because
					// the backoff has exceeded a maximum allowable value.
					log.Infof("Blacklisting %q due to errors", oq.destination)
					return
				}
			} else if sent > 0 {
				// If we successfully sent the invites then clear out
				// the pending invites.
				oq.statistics.Success()
				// Reallocate so that the underlying array can be GC'd, as
				// opposed to growing forever.
				oq.cleanPendingInvites()
			}
		}
	}
}

// cleanPendingInvites cleans out the pending invite buffer,
// removing all references so that the underlying objects can
// be GC'd.
func (oq *destinationQueue) cleanPendingInvites() {
	for i := 0; i < len(oq.pendingInvites); i++ {
		oq.pendingInvites[i] = nil
	}
	oq.pendingInvites = []*gomatrixserverlib.InviteV2Request{}
}

// nextTransaction creates a new transaction from the pending event
// queue and sends it. Returns true if a transaction was sent or
// false otherwise.
// nolint:gocyclo
func (oq *destinationQueue) nextTransaction() (bool, error) {
	// Before we do anything, we need to roll over the transaction
	// ID that is being used to coalesce events into the next TX.
	// Otherwise it's possible that we'll pick up an incomplete
	// transaction and end up nuking the rest of the events at the
	// cleanup stage.
	oq.transactionIDMutex.Lock()
	oq.transactionID = ""
	oq.transactionIDMutex.Unlock()
	oq.transactionCount.Store(0)

	// Create the transaction.
	t := gomatrixserverlib.Transaction{
		PDUs: []json.RawMessage{},
		EDUs: []gomatrixserverlib.EDU{},
	}
	t.Origin = oq.origin
	t.Destination = oq.destination
	t.OriginServerTS = gomatrixserverlib.AsTimestamp(time.Now())

	// Ask the database for any pending PDUs from the next transaction.
	// maxPDUsPerTransaction is an upper limit but we probably won't
	// actually retrieve that many events.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	txid, pdus, pduReceipt, err := oq.db.GetNextTransactionPDUs(
		ctx,                   // context
		oq.destination,        // server name
		maxPDUsPerTransaction, // max events to retrieve
	)
	if err != nil {
		log.WithError(err).Errorf("failed to get next transaction PDUs for server %q", oq.destination)
		return false, fmt.Errorf("oq.db.GetNextTransactionPDUs: %w", err)
	}

	edus, eduReceipt, err := oq.db.GetNextTransactionEDUs(
		ctx,                   // context
		oq.destination,        // server name
		maxEDUsPerTransaction, // max events to retrieve
	)
	if err != nil {
		log.WithError(err).Errorf("failed to get next transaction EDUs for server %q", oq.destination)
		return false, fmt.Errorf("oq.db.GetNextTransactionEDUs: %w", err)
	}

	// If we didn't get anything from the database and there are no
	// pending EDUs then there's nothing to do - stop here.
	if len(pdus) == 0 && len(edus) == 0 {
		return false, nil
	}

	// Pick out the transaction ID from the database. If we didn't
	// get a transaction ID (i.e. because there are no PDUs but only
	// EDUs) then generate a transaction ID.
	t.TransactionID = txid
	if t.TransactionID == "" {
		now := gomatrixserverlib.AsTimestamp(time.Now())
		t.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, oq.statistics.SuccessCount()))
	}

	// Go through PDUs that we retrieved from the database, if any,
	// and add them into the transaction.
	for _, pdu := range pdus {
		// Append the JSON of the event, since this is a json.RawMessage type in the
		// gomatrixserverlib.Transaction struct
		t.PDUs = append(t.PDUs, (*pdu).JSON())
	}

	// Do the same for pending EDUS in the queue.
	for _, edu := range edus {
		t.EDUs = append(t.EDUs, *edu)
	}

	logrus.WithField("server_name", oq.destination).Infof("Sending transaction %q containing %d PDUs, %d EDUs", t.TransactionID, len(t.PDUs), len(t.EDUs))

	// Try to send the transaction to the destination server.
	// TODO: we should check for 500-ish fails vs 400-ish here,
	// since we shouldn't queue things indefinitely in response
	// to a 400-ish error
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	_, err = oq.client.SendTransaction(ctx, t)
	switch err.(type) {
	case nil:
		// Clean up the transaction in the database.
		if pduReceipt != nil {
			//logrus.Infof("Cleaning PDUs %q", pduReceipt.String())
			if err = oq.db.CleanPDUs(context.Background(), oq.destination, pduReceipt); err != nil {
				log.WithError(err).Errorf("failed to clean PDUs %q for server %q", pduReceipt.String(), t.Destination)
			}
		}
		if eduReceipt != nil {
			//logrus.Infof("Cleaning EDUs %q", eduReceipt.String())
			if err = oq.db.CleanEDUs(context.Background(), oq.destination, eduReceipt); err != nil {
				log.WithError(err).Errorf("failed to clean EDUs %q for server %q", eduReceipt.String(), t.Destination)
			}
		}
		return true, nil
	case gomatrix.HTTPError:
		// Report that we failed to send the transaction and we
		// will retry again, subject to backoff.
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

		invEv := inviteRes.Event.Sign(string(oq.signing.ServerName), oq.signing.KeyID, oq.signing.PrivateKey).Headered(roomVersion)
		_, err = api.SendEvents(context.TODO(), oq.rsAPI, []gomatrixserverlib.HeaderedEvent{invEv}, oq.signing.ServerName, nil)
		if err != nil {
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
