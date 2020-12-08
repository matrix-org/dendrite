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
	queueIdleTimeout = time.Second * 30
)

// destinationQueue is a queue of events for a single destination.
// It is responsible for sending the events to the destination and
// ensures that only one request is in flight to a given destination
// at a time.
type destinationQueue struct {
	db                  storage.Database
	signing             *SigningInfo
	rsAPI               api.RoomserverInternalAPI
	client              *gomatrixserverlib.FederationClient // federation client
	origin              gomatrixserverlib.ServerName        // origin of requests
	destination         gomatrixserverlib.ServerName        // destination of requests
	running             atomic.Bool                         // is the queue worker running?
	backingOff          atomic.Bool                         // true if we're backing off
	overflowed          atomic.Bool                         // the queues exceed maxPDUsInMemory/maxEDUsInMemory, so we should consult the database for more
	transactionID       gomatrixserverlib.TransactionID     // the ID to commit to the database with
	transactionIDMutex  sync.RWMutex                        // protects transactionID
	transactionPDUCount atomic.Int32                        // how many PDUs in database transaction?
	transactionEDUCount atomic.Int32                        // how many EDUs in database transaction?
	statistics          *statistics.ServerStatistics        // statistics about this remote server
	notify              chan struct{}                       // interrupts idle wait pending PDUs/EDUs
	pendingTransactions queuedTransactions                  // transactions waiting to be sent
	pendingMutex        sync.RWMutex                        // protects pendingTransactions
	interruptBackoff    chan bool                           // interrupts backoff
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(event *gomatrixserverlib.HeaderedEvent, receipt *shared.Receipt) {
	if event == nil {
		log.Errorf("attempt to send nil PDU with destination %q", oq.destination)
		return
	}

	// Try to queue the PDU up in memory. If there was enough free
	// space then we'll get a transaction ID back.
	oq.pendingMutex.Lock()
	transactionID := oq.pendingTransactions.queuePDUs(receipt, event)
	oq.pendingMutex.Unlock()

	// Check if we got a transaction ID back.
	if transactionID == "" {
		// If we hit this point then we weren't able to fit the event
		// into the memory cache, therefore we need to generate a new
		// transaction ID to commit to the database. If we don't have
		// a transaction ID for the database, or we've exceeded the
		// number of PDUs we can fit in the last one, generate a new
		// one.
		oq.transactionIDMutex.Lock()
		if oq.transactionID == "" || oq.transactionPDUCount.Load() > maxPDUsPerTransaction {
			now := gomatrixserverlib.AsTimestamp(time.Now())
			oq.transactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, oq.statistics.SuccessCount()))
			oq.transactionPDUCount.Store(0)
			oq.transactionEDUCount.Store(0)
			transactionID = oq.transactionID
		}
		oq.transactionIDMutex.Unlock()
		oq.overflowed.Store(true)
	}

	// Create a database entry that associates the given PDU NID with
	// this destination queue. We'll then be able to retrieve the PDU
	// later.
	if err := oq.db.AssociatePDUWithDestination(
		context.TODO(),
		transactionID,  // the transaction ID
		oq.destination, // the destination server name
		receipt,        // NIDs from federationsender_queue_json table
	); err != nil {
		log.WithError(err).Errorf("failed to associate PDU %q with destination %q", event.EventID(), oq.destination)
		return
	}

	// We've successfully added a PDU to the transaction so increase
	// the counter.
	oq.transactionPDUCount.Add(1)

	// Wake up the queue.
	oq.wakeQueueIfNeeded()
	select {
	case oq.notify <- struct{}{}:
	default:
	}
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEDU(event *gomatrixserverlib.EDU, receipt *shared.Receipt) {
	if event == nil {
		log.Errorf("attempt to send nil EDU with destination %q", oq.destination)
		return
	}

	// Try to queue the PDU up in memory. If there was enough free
	// space then we'll get a transaction ID back.
	oq.pendingMutex.Lock()
	transactionID := oq.pendingTransactions.queueEDUs(receipt, event)
	oq.pendingMutex.Unlock()

	// Check if we got a transaction ID back.
	if transactionID == "" {
		// If we hit this point then we weren't able to fit the event
		// into the memory cache, therefore we need to generate a new
		// transaction ID to commit to the database. If we don't have
		// a transaction ID for the database, or we've exceeded the
		// number of PDUs we can fit in the last one, generate a new
		// one.
		/*
			oq.transactionIDMutex.Lock()
			if oq.transactionID == "" || oq.transactionPDUCount.Load() > maxPDUsPerTransaction {
				now := gomatrixserverlib.AsTimestamp(time.Now())
				oq.transactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, oq.statistics.SuccessCount()))
				oq.transactionPDUCount.Store(0)
				oq.transactionEDUCount.Store(0)
				transactionID = oq.transactionID
			}
			oq.transactionIDMutex.Unlock()
		*/
		oq.overflowed.Store(true)
	}

	// Create a database entry that associates the given EDU NID with
	// this destination queue. We'll then be able to retrieve the EDU
	// later.
	if err := oq.db.AssociateEDUWithDestination(
		context.TODO(),
		//transactionID,  // the transaction ID
		oq.destination, // the destination server name
		receipt,        // NIDs from federationsender_queue_json table
	); err != nil {
		log.WithError(err).Errorf("failed to associate EDU with destination %q", oq.destination)
		return
	}

	// We've successfully added a PDU to the transaction so increase
	// the counter.
	oq.transactionEDUCount.Add(1)

	// Wake up the queue.
	oq.wakeQueueIfNeeded()
	select {
	case oq.notify <- struct{}{}:
	default:
	}
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

// getNextTransactionFromDatabase will look at the database and see
// if there are any persisted events that haven't been sent to this
// destination yet. If so, they will be queued up.
// nolint:gocyclo
func (oq *destinationQueue) getNextTransactionFromDatabase() {
	// Check to see if there's anything to do for this server
	// in the database.
	ctx := context.Background()
	oq.pendingMutex.Lock()
	defer oq.pendingMutex.Unlock()

	transactionID, pdus, pduReceipt, err := oq.db.GetNextTransactionPDUs(ctx, oq.destination, maxPDUsPerTransaction)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to get pending PDUs for %q", oq.destination)
	}

	edus, eduReceipt, err := oq.db.GetNextTransactionEDUs(ctx, oq.destination, maxEDUsPerTransaction)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to get pending EDUs for %q", oq.destination)
	}

	oq.pendingTransactions.createNew(transactionID)
	oq.pendingTransactions.queuePDUs(pduReceipt, pdus...)
	oq.pendingTransactions.queueEDUs(eduReceipt, edus...)

	// If we've retrieved some events then notify the destination queue goroutine.
	if len(pdus) > 0 || len(edus) > 0 {
		select {
		case oq.notify <- struct{}{}:
		default:
		}
	}
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

	// Mark the queue as overflowed, so we will consult the database
	// to see if there's anything new to send.
	oq.overflowed.Store(true)

	for {
		// If we are overflowing memory and have sent things out to the
		// database then we can look up what those things are.
		if oq.overflowed.Load() {
			oq.getNextTransactionFromDatabase()
		}

		// If we have nothing to do then wait either for incoming events, or
		// until we hit an idle timeout.
		select {
		case <-oq.notify:
			// There's work to do, either because getPendingFromDatabase
			// told us there is, or because a new event has come in via
			// sendEvent/sendEDU.
		case <-time.After(queueIdleTimeout):
			// The worker is idle so stop the goroutine. It'll get
			// restarted automatically the next time we have an event to
			// send.
			return
		}

		// If we are backing off this server then wait for the
		// backoff duration to complete first, or until explicitly
		// told to retry.
		until, blacklisted := oq.statistics.BackoffInfo()
		if blacklisted {
			// It's been suggested that we should give up because the backoff
			// has exceeded a maximum allowable value. Clean up the in-memory
			// buffers at this point. The PDU clean-up is already on a defer.
			log.Warnf("Blacklisting %q due to exceeding backoff threshold", oq.destination)
			oq.pendingMutex.Lock()
			for i := range oq.pendingTransactions.queue {
				oq.pendingTransactions.queue[i] = nil
			}
			oq.pendingTransactions.queue = nil
			oq.pendingMutex.Lock()
			return
		}
		if until != nil && until.After(time.Now()) {
			// We haven't backed off yet, so wait for the suggested amount of
			// time.
			duration := time.Until(*until)
			log.Warnf("Backing off %q for %s", oq.destination, duration)
			select {
			case <-time.After(duration):
			case <-oq.interruptBackoff:
			}
		}

		// Work out which PDUs/EDUs to include in the next transaction.
		oq.pendingMutex.RLock()
		if len(oq.pendingTransactions.queue) == 0 {
			continue
		}
		next := oq.pendingTransactions.queue[0]
		oq.pendingMutex.RUnlock()

		// If we have pending PDUs or EDUs then construct a transaction.
		// Try sending the next transaction and see what happens.
		transaction, _, _, terr := oq.nextTransaction(next)
		if terr != nil {
			// We failed to send the transaction. Mark it as a failure.
			oq.statistics.Failure()

		} else if transaction {
			// If we successfully sent the transaction then clear out
			// the pending events and EDUs, and wipe our transaction ID.
			oq.statistics.Success()
			oq.pendingMutex.Lock()
			for i := range next.pdus {
				next.pdus[i] = nil
			}
			for i := range next.edus {
				next.edus[i] = nil
			}
			oq.pendingTransactions.queue = oq.pendingTransactions.queue[1:]
			oq.pendingMutex.Unlock()
		}
	}
}

// nextTransaction creates a new transaction from the pending event
// queue and sends it. Returns true if a transaction was sent or
// false otherwise.
// nolint:gocyclo
func (oq *destinationQueue) nextTransaction(transaction *queuedTransaction) (bool, int, int, error) {
	// If there's no projected transaction ID then generate one. If
	// the transaction succeeds then we'll set it back to "" so that
	// we generate a new one next time. If it fails, we'll preserve
	// it so that we retry with the same transaction ID.
	oq.transactionIDMutex.Lock()
	if oq.transactionID == "" {
		now := gomatrixserverlib.AsTimestamp(time.Now())
		oq.transactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, oq.statistics.SuccessCount()))
	}
	oq.transactionIDMutex.Unlock()

	// Create the transaction.
	t := gomatrixserverlib.Transaction{
		PDUs: []json.RawMessage{},
		EDUs: []gomatrixserverlib.EDU{},
	}
	t.Origin = oq.origin
	t.Destination = oq.destination
	t.OriginServerTS = gomatrixserverlib.AsTimestamp(time.Now())
	t.TransactionID = oq.transactionID

	// If we didn't get anything from the database and there are no
	// pending EDUs then there's nothing to do - stop here.
	if len(transaction.pdus) == 0 && len(transaction.edus) == 0 {
		return false, 0, 0, nil
	}

	// Go through PDUs that we retrieved from the database, if any,
	// and add them into the transaction.
	for _, pdu := range transaction.pdus {
		if pdu == nil {
			continue
		}
		// Append the JSON of the event, since this is a json.RawMessage type in the
		// gomatrixserverlib.Transaction struct
		t.PDUs = append(t.PDUs, pdu.JSON())
	}

	// Do the same for pending EDUS in the queue.
	for _, edu := range transaction.edus {
		if edu == nil {
			continue
		}
		t.EDUs = append(t.EDUs, *edu)
	}

	logrus.WithField("server_name", oq.destination).Debugf("Sending transaction %q containing %d PDUs, %d EDUs", t.TransactionID, len(t.PDUs), len(t.EDUs))

	// Try to send the transaction to the destination server.
	// TODO: we should check for 500-ish fails vs 400-ish here,
	// since we shouldn't queue things indefinitely in response
	// to a 400-ish error
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	_, err := oq.client.SendTransaction(ctx, t)
	switch err.(type) {
	case nil:
		// Clean up the transaction in the database.
		for _, receipt := range transaction.pduReceipts {
			//logrus.Infof("Cleaning PDUs %q", pduReceipt.String())
			if err = oq.db.CleanPDUs(context.Background(), oq.destination, receipt); err != nil {
				log.WithError(err).Errorf("Failed to clean PDUs for server %q", t.Destination)
			}
		}
		for _, receipt := range transaction.eduReceipts {
			//logrus.Infof("Cleaning EDUs %q", eduReceipt.String())
			if err = oq.db.CleanEDUs(context.Background(), oq.destination, receipt); err != nil {
				log.WithError(err).Errorf("Failed to clean EDUs for server %q", t.Destination)
			}
		}
		// Reset the transaction ID.
		oq.transactionIDMutex.Lock()
		oq.transactionID = ""
		oq.transactionIDMutex.Unlock()
		return true, len(t.PDUs), len(t.EDUs), nil
	case gomatrix.HTTPError:
		// Report that we failed to send the transaction and we
		// will retry again, subject to backoff.
		return false, 0, 0, err
	default:
		log.WithFields(log.Fields{
			"destination": oq.destination,
			log.ErrorKey:  err,
		}).Infof("Failed to send transaction %q", t.TransactionID)
		return false, 0, 0, err
	}
}
