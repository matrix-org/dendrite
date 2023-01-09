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

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"

	fedapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/process"
)

const (
	maxPDUsPerTransaction = 50
	maxEDUsPerTransaction = 100
	maxPDUsInMemory       = 128
	maxEDUsInMemory       = 128
	queueIdleTimeout      = time.Second * 30
)

// destinationQueue is a queue of events for a single destination.
// It is responsible for sending the events to the destination and
// ensures that only one request is in flight to a given destination
// at a time.
type destinationQueue struct {
	queues             *OutgoingQueues
	db                 storage.Database
	process            *process.ProcessContext
	signing            map[gomatrixserverlib.ServerName]*gomatrixserverlib.SigningIdentity
	rsAPI              api.FederationRoomserverAPI
	client             fedapi.FederationClient         // federation client
	origin             gomatrixserverlib.ServerName    // origin of requests
	destination        gomatrixserverlib.ServerName    // destination of requests
	running            atomic.Bool                     // is the queue worker running?
	backingOff         atomic.Bool                     // true if we're backing off
	overflowed         atomic.Bool                     // the queues exceed maxPDUsInMemory/maxEDUsInMemory, so we should consult the database for more
	statistics         *statistics.ServerStatistics    // statistics about this remote server
	transactionIDMutex sync.Mutex                      // protects transactionID
	transactionID      gomatrixserverlib.TransactionID // last transaction ID if retrying, or "" if last txn was successful
	notify             chan struct{}                   // interrupts idle wait pending PDUs/EDUs
	pendingPDUs        []*queuedPDU                    // PDUs waiting to be sent
	pendingEDUs        []*queuedEDU                    // EDUs waiting to be sent
	pendingMutex       sync.RWMutex                    // protects pendingPDUs and pendingEDUs
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(event *gomatrixserverlib.HeaderedEvent, receipt *shared.Receipt) {
	if event == nil {
		logrus.Errorf("attempt to send nil PDU with destination %q", oq.destination)
		return
	}

	// Check if the destination is blacklisted. If it isn't then wake
	// up the queue.
	if !oq.statistics.Blacklisted() {
		// If there's room in memory to hold the event then add it to the
		// list.
		oq.pendingMutex.Lock()
		if len(oq.pendingPDUs) < maxPDUsInMemory {
			oq.pendingPDUs = append(oq.pendingPDUs, &queuedPDU{
				pdu:     event,
				receipt: receipt,
			})
		} else {
			oq.overflowed.Store(true)
		}
		oq.pendingMutex.Unlock()

		if !oq.backingOff.Load() {
			oq.wakeQueueAndNotify()
		}
	}
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEDU(event *gomatrixserverlib.EDU, receipt *shared.Receipt) {
	if event == nil {
		logrus.Errorf("attempt to send nil EDU with destination %q", oq.destination)
		return
	}

	// Check if the destination is blacklisted. If it isn't then wake
	// up the queue.
	if !oq.statistics.Blacklisted() {
		// If there's room in memory to hold the event then add it to the
		// list.
		oq.pendingMutex.Lock()
		if len(oq.pendingEDUs) < maxEDUsInMemory {
			oq.pendingEDUs = append(oq.pendingEDUs, &queuedEDU{
				edu:     event,
				receipt: receipt,
			})
		} else {
			oq.overflowed.Store(true)
		}
		oq.pendingMutex.Unlock()

		if !oq.backingOff.Load() {
			oq.wakeQueueAndNotify()
		}
	}
}

// handleBackoffNotifier is registered as the backoff notification
// callback with Statistics. It will wakeup and notify the queue
// if the queue is currently backing off.
func (oq *destinationQueue) handleBackoffNotifier() {
	// Only wake up the queue if it is backing off.
	// Otherwise there is no pending work for the queue to handle
	// so waking the queue would be a waste of resources.
	if oq.backingOff.Load() {
		oq.wakeQueueAndNotify()
	}
}

// wakeQueueIfEventsPending calls wakeQueueAndNotify only if there are
// pending events or if forceWakeup is true. This prevents starting the
// queue unnecessarily.
func (oq *destinationQueue) wakeQueueIfEventsPending(forceWakeup bool) {
	eventsPending := func() bool {
		oq.pendingMutex.Lock()
		defer oq.pendingMutex.Unlock()
		return len(oq.pendingPDUs) > 0 || len(oq.pendingEDUs) > 0
	}

	// NOTE : Only wakeup and notify the queue if there are pending events
	// or if forceWakeup is true. Otherwise there is no reason to start the
	// queue goroutine and waste resources.
	if forceWakeup || eventsPending() {
		logrus.Info("Starting queue due to pending events or forceWakeup")
		oq.wakeQueueAndNotify()
	}
}

// wakeQueueAndNotify ensures the destination queue is running and notifies it
// that there is pending work.
func (oq *destinationQueue) wakeQueueAndNotify() {
	// NOTE : Send notification before waking queue to prevent a race
	// where the queue was running and stops due to a timeout in between
	// checking it and sending the notification.

	// Notify the queue that there are events ready to send.
	select {
	case oq.notify <- struct{}{}:
	default:
	}

	// Wake up the queue if it's asleep.
	oq.wakeQueueIfNeeded()
}

// wakeQueueIfNeeded will wake up the destination queue if it is
// not already running.
func (oq *destinationQueue) wakeQueueIfNeeded() {
	// Clear the backingOff flag and update the backoff metrics if it was set.
	if oq.backingOff.CompareAndSwap(true, false) {
		destinationQueueBackingOff.Dec()
	}

	// If we aren't running then wake up the queue.
	if !oq.running.Load() {
		// Start the queue.
		go oq.backgroundSend()
	}
}

// getPendingFromDatabase will look at the database and see if
// there are any persisted events that haven't been sent to this
// destination yet. If so, they will be queued up.
func (oq *destinationQueue) getPendingFromDatabase() {
	// Check to see if there's anything to do for this server
	// in the database.
	retrieved := false
	ctx := oq.process.Context()
	oq.pendingMutex.Lock()
	defer oq.pendingMutex.Unlock()

	// Take a note of all of the PDUs and EDUs that we already
	// have cached. We will index them based on the receipt,
	// which ultimately just contains the index of the PDU/EDU
	// in the database.
	gotPDUs := map[string]struct{}{}
	gotEDUs := map[string]struct{}{}
	for _, pdu := range oq.pendingPDUs {
		gotPDUs[pdu.receipt.String()] = struct{}{}
	}
	for _, edu := range oq.pendingEDUs {
		gotEDUs[edu.receipt.String()] = struct{}{}
	}

	overflowed := false
	if pduCapacity := maxPDUsInMemory - len(oq.pendingPDUs); pduCapacity > 0 {
		// We have room in memory for some PDUs - let's request no more than that.
		if pdus, err := oq.db.GetPendingPDUs(ctx, oq.destination, maxPDUsInMemory); err == nil {
			if len(pdus) == maxPDUsInMemory {
				overflowed = true
			}
			for receipt, pdu := range pdus {
				if _, ok := gotPDUs[receipt.String()]; ok {
					continue
				}
				oq.pendingPDUs = append(oq.pendingPDUs, &queuedPDU{receipt, pdu})
				retrieved = true
				if len(oq.pendingPDUs) == maxPDUsInMemory {
					break
				}
			}
		} else {
			logrus.WithError(err).Errorf("Failed to get pending PDUs for %q", oq.destination)
		}
	}

	if eduCapacity := maxEDUsInMemory - len(oq.pendingEDUs); eduCapacity > 0 {
		// We have room in memory for some EDUs - let's request no more than that.
		if edus, err := oq.db.GetPendingEDUs(ctx, oq.destination, maxEDUsInMemory); err == nil {
			if len(edus) == maxEDUsInMemory {
				overflowed = true
			}
			for receipt, edu := range edus {
				if _, ok := gotEDUs[receipt.String()]; ok {
					continue
				}
				oq.pendingEDUs = append(oq.pendingEDUs, &queuedEDU{receipt, edu})
				retrieved = true
				if len(oq.pendingEDUs) == maxEDUsInMemory {
					break
				}
			}
		} else {
			logrus.WithError(err).Errorf("Failed to get pending EDUs for %q", oq.destination)
		}
	}

	// If we've retrieved all of the events from the database with room to spare
	// in memory then we'll no longer consider this queue to be overflowed.
	if !overflowed {
		oq.overflowed.Store(false)
	} else {
	}
	// If we've retrieved some events then notify the destination queue goroutine.
	if retrieved {
		select {
		case oq.notify <- struct{}{}:
		default:
		}
	}
}

// checkNotificationsOnClose checks for any remaining notifications
// and starts a new backgroundSend goroutine if any exist.
func (oq *destinationQueue) checkNotificationsOnClose() {
	// NOTE : If we are stopping the queue due to blacklist then it
	// doesn't matter if we have been notified of new work since
	// this queue instance will be deleted anyway.
	if !oq.statistics.Blacklisted() {
		select {
		case <-oq.notify:
			// We received a new notification in between the
			// idle timeout firing and stopping the goroutine.
			// Immediately restart the queue.
			oq.wakeQueueAndNotify()
		default:
		}
	}
}

// backgroundSend is the worker goroutine for sending events.
func (oq *destinationQueue) backgroundSend() {
	// Check if a worker is already running, and if it isn't, then
	// mark it as started.
	if !oq.running.CompareAndSwap(false, true) {
		return
	}

	// Register queue cleanup functions.
	// NOTE : The ordering here is very intentional.
	defer oq.checkNotificationsOnClose()
	defer oq.running.Store(false)

	destinationQueueRunning.Inc()
	defer destinationQueueRunning.Dec()

	idleTimeout := time.NewTimer(queueIdleTimeout)
	defer idleTimeout.Stop()

	// Mark the queue as overflowed, so we will consult the database
	// to see if there's anything new to send.
	oq.overflowed.Store(true)

	for {
		// If we are overflowing memory and have sent things out to the
		// database then we can look up what those things are.
		if oq.overflowed.Load() {
			oq.getPendingFromDatabase()
		}

		// Reset the queue idle timeout.
		if !idleTimeout.Stop() {
			select {
			case <-idleTimeout.C:
			default:
			}
		}
		idleTimeout.Reset(queueIdleTimeout)

		// If we have nothing to do then wait either for incoming events, or
		// until we hit an idle timeout.
		select {
		case <-oq.notify:
			// There's work to do, either because getPendingFromDatabase
			// told us there is, a new event has come in via sendEvent/sendEDU,
			// or we are backing off and it is time to retry.
		case <-idleTimeout.C:
			// The worker is idle so stop the goroutine. It'll get
			// restarted automatically the next time we have an event to
			// send.
			return
		case <-oq.process.Context().Done():
			// The parent process is shutting down, so stop.
			oq.statistics.ClearBackoff()
			return
		}

		// Work out which PDUs/EDUs to include in the next transaction.
		oq.pendingMutex.RLock()
		pduCount := len(oq.pendingPDUs)
		eduCount := len(oq.pendingEDUs)
		if pduCount > maxPDUsPerTransaction {
			pduCount = maxPDUsPerTransaction
		}
		if eduCount > maxEDUsPerTransaction {
			eduCount = maxEDUsPerTransaction
		}
		toSendPDUs := oq.pendingPDUs[:pduCount]
		toSendEDUs := oq.pendingEDUs[:eduCount]
		oq.pendingMutex.RUnlock()

		// If we didn't get anything from the database and there are no
		// pending EDUs then there's nothing to do - stop here.
		if pduCount == 0 && eduCount == 0 {
			continue
		}

		// If we have pending PDUs or EDUs then construct a transaction.
		// Try sending the next transaction and see what happens.
		terr := oq.nextTransaction(toSendPDUs, toSendEDUs)
		if terr != nil {
			// We failed to send the transaction. Mark it as a failure.
			_, blacklisted := oq.statistics.Failure()
			if !blacklisted {
				// Register the backoff state and exit the goroutine.
				// It'll get restarted automatically when the backoff
				// completes.
				oq.backingOff.Store(true)
				destinationQueueBackingOff.Inc()
				return
			} else {
				// Immediately trigger the blacklist logic.
				oq.blacklistDestination()
				return
			}
		} else {
			oq.handleTransactionSuccess(pduCount, eduCount)
		}
	}
}

// nextTransaction creates a new transaction from the pending event
// queue and sends it.
// Returns an error if the transaction wasn't sent.
func (oq *destinationQueue) nextTransaction(
	pdus []*queuedPDU,
	edus []*queuedEDU,
) error {
	// Create the transaction.
	t, pduReceipts, eduReceipts := oq.createTransaction(pdus, edus)
	logrus.WithField("server_name", oq.destination).Debugf("Sending transaction %q containing %d PDUs, %d EDUs", t.TransactionID, len(t.PDUs), len(t.EDUs))

	// Try to send the transaction to the destination server.
	ctx, cancel := context.WithTimeout(oq.process.Context(), time.Minute*5)
	defer cancel()
	_, err := oq.client.SendTransaction(ctx, t)
	switch errResponse := err.(type) {
	case nil:
		// Clean up the transaction in the database.
		if pduReceipts != nil {
			//logrus.Infof("Cleaning PDUs %q", pduReceipt.String())
			if err = oq.db.CleanPDUs(oq.process.Context(), oq.destination, pduReceipts); err != nil {
				logrus.WithError(err).Errorf("Failed to clean PDUs for server %q", t.Destination)
			}
		}
		if eduReceipts != nil {
			//logrus.Infof("Cleaning EDUs %q", eduReceipt.String())
			if err = oq.db.CleanEDUs(oq.process.Context(), oq.destination, eduReceipts); err != nil {
				logrus.WithError(err).Errorf("Failed to clean EDUs for server %q", t.Destination)
			}
		}
		// Reset the transaction ID.
		oq.transactionIDMutex.Lock()
		oq.transactionID = ""
		oq.transactionIDMutex.Unlock()
		return nil
	case gomatrix.HTTPError:
		// Report that we failed to send the transaction and we
		// will retry again, subject to backoff.

		// TODO: we should check for 500-ish fails vs 400-ish here,
		// since we shouldn't queue things indefinitely in response
		// to a 400-ish error
		code := errResponse.Code
		logrus.Debug("Transaction failed with HTTP", code)
		return err
	default:
		logrus.WithFields(logrus.Fields{
			"destination":   oq.destination,
			logrus.ErrorKey: err,
		}).Debugf("Failed to send transaction %q", t.TransactionID)
		return err
	}
}

// createTransaction generates a gomatrixserverlib.Transaction from the provided pdus and edus.
// It also returns the associated event receipts so they can be cleaned from the database in
// the case of a successful transaction.
func (oq *destinationQueue) createTransaction(
	pdus []*queuedPDU,
	edus []*queuedEDU,
) (gomatrixserverlib.Transaction, []*shared.Receipt, []*shared.Receipt) {
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

	t := gomatrixserverlib.Transaction{
		PDUs: []json.RawMessage{},
		EDUs: []gomatrixserverlib.EDU{},
	}
	t.Origin = oq.origin
	t.Destination = oq.destination
	t.OriginServerTS = gomatrixserverlib.AsTimestamp(time.Now())
	t.TransactionID = oq.transactionID

	var pduReceipts []*shared.Receipt
	var eduReceipts []*shared.Receipt

	// Go through PDUs that we retrieved from the database, if any,
	// and add them into the transaction.
	for _, pdu := range pdus {
		// These should never be nil.
		if pdu == nil || pdu.pdu == nil {
			continue
		}
		// Append the JSON of the event, since this is a json.RawMessage type in the
		// gomatrixserverlib.Transaction struct
		t.PDUs = append(t.PDUs, pdu.pdu.JSON())
		pduReceipts = append(pduReceipts, pdu.receipt)
	}

	// Do the same for pending EDUS in the queue.
	for _, edu := range edus {
		// These should never be nil.
		if edu == nil || edu.edu == nil {
			continue
		}
		t.EDUs = append(t.EDUs, *edu.edu)
		eduReceipts = append(eduReceipts, edu.receipt)
	}

	return t, pduReceipts, eduReceipts
}

// blacklistDestination removes all pending PDUs and EDUs that have been cached
// and deletes this queue.
func (oq *destinationQueue) blacklistDestination() {
	// It's been suggested that we should give up because the backoff
	// has exceeded a maximum allowable value. Clean up the in-memory
	// buffers at this point. The PDU clean-up is already on a defer.
	logrus.Warnf("Blacklisting %q due to exceeding backoff threshold", oq.destination)

	oq.pendingMutex.Lock()
	for i := range oq.pendingPDUs {
		oq.pendingPDUs[i] = nil
	}
	for i := range oq.pendingEDUs {
		oq.pendingEDUs[i] = nil
	}
	oq.pendingPDUs = nil
	oq.pendingEDUs = nil
	oq.pendingMutex.Unlock()

	// Delete this queue as no more messages will be sent to this
	// destination until it is no longer blacklisted.
	oq.statistics.AssignBackoffNotifier(nil)
	oq.queues.clearQueue(oq)
}

// handleTransactionSuccess updates the cached event queues as well as the success and
// backoff information for this server.
func (oq *destinationQueue) handleTransactionSuccess(pduCount int, eduCount int) {
	// If we successfully sent the transaction then clear out
	// the pending events and EDUs, and wipe our transaction ID.
	oq.statistics.Success()
	oq.pendingMutex.Lock()
	defer oq.pendingMutex.Unlock()

	for i := range oq.pendingPDUs[:pduCount] {
		oq.pendingPDUs[i] = nil
	}
	for i := range oq.pendingEDUs[:eduCount] {
		oq.pendingEDUs[i] = nil
	}
	oq.pendingPDUs = oq.pendingPDUs[pduCount:]
	oq.pendingEDUs = oq.pendingEDUs[eduCount:]

	if len(oq.pendingPDUs) > 0 || len(oq.pendingEDUs) > 0 {
		select {
		case oq.notify <- struct{}{}:
		default:
		}
	}
}
