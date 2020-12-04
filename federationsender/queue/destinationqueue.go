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
	client             *gomatrixserverlib.FederationClient // federation client
	origin             gomatrixserverlib.ServerName        // origin of requests
	destination        gomatrixserverlib.ServerName        // destination of requests
	running            atomic.Bool                         // is the queue worker running?
	backingOff         atomic.Bool                         // true if we're backing off
	statistics         *statistics.ServerStatistics        // statistics about this remote server
	transactionIDMutex sync.Mutex                          // protects transactionID
	transactionID      gomatrixserverlib.TransactionID     // last transaction ID
	transactionCount   atomic.Int32                        // how many events in this transaction so far
	notifyPDUs         chan *queuedPDU                     // interrupts idle wait for PDUs
	notifyEDUs         chan *queuedEDU                     // interrupts idle wait for EDUs
	interruptBackoff   chan bool                           // interrupts backoff
}

// Send event adds the event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEvent(event *gomatrixserverlib.HeaderedEvent, receipt *shared.Receipt) {
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
		log.WithError(err).Errorf("failed to associate PDU %q with destination %q", event.EventID(), oq.destination)
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
		case oq.notifyPDUs <- &queuedPDU{
			receipt: receipt,
			pdu:     event,
		}:
		default:
		}
	}
}

// sendEDU adds the EDU event to the pending queue for the destination.
// If the queue is empty then it starts a background goroutine to
// start sending events to that destination.
func (oq *destinationQueue) sendEDU(event *gomatrixserverlib.EDU, receipt *shared.Receipt) {
	// Create a database entry that associates the given PDU NID with
	// this destination queue. We'll then be able to retrieve the PDU
	// later.
	if err := oq.db.AssociateEDUWithDestination(
		context.TODO(),
		oq.destination, // the destination server name
		receipt,        // NIDs from federationsender_queue_json table
	); err != nil {
		log.WithError(err).Errorf("failed to associate EDU with destination %q", oq.destination)
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
		case oq.notifyEDUs <- &queuedEDU{
			receipt: receipt,
			edu:     event,
		}:
		default:
		}
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

		// Check to see if there's anything to do for this server
		// in the database.
		ctx := context.Background()
		go func(ctx context.Context) {
			if pdus, err := oq.db.GetPendingPDUs(ctx, oq.destination, maxPDUsPerTransaction); err == nil {
				for receipt, pdu := range pdus {
					select {
					case oq.notifyPDUs <- &queuedPDU{receipt, pdu}:
					default:
						return
					}
				}
			}
		}(ctx)
		go func(ctx context.Context) {
			if edus, err := oq.db.GetPendingEDUs(ctx, oq.destination, maxEDUsPerTransaction); err == nil {
				for receipt, edu := range edus {
					select {
					case oq.notifyEDUs <- &queuedEDU{receipt, edu}:
					default:
						return
					}
				}
			}
		}(ctx)
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

	pendingPDUs := []*queuedPDU{}
	pendingEDUs := []*queuedEDU{}

	for {
		// If we have nothing to do then wait either for incoming events, or
		// until we hit an idle timeout.
		select {
		case pdu := <-oq.notifyPDUs:
			// We were woken up because there are new PDUs waiting in the
			// database.
			pendingPDUs = append(pendingPDUs, pdu)
		pendingPDULoop:
			for i := 1; i < maxPDUsPerTransaction; i++ {
				select {
				case edu := <-oq.notifyEDUs:
					pendingEDUs = append(pendingEDUs, edu)
				default:
					break pendingPDULoop
				}
			}

		case edu := <-oq.notifyEDUs:
			// We were woken up because there are new PDUs waiting in the
			// database.
			pendingEDUs = append(pendingEDUs, edu)
		pendingEDULoop:
			for i := 1; i < maxEDUsPerTransaction; i++ {
				select {
				case edu := <-oq.notifyEDUs:
					pendingEDUs = append(pendingEDUs, edu)
				default:
					break pendingEDULoop
				}
			}

		case <-time.After(queueIdleTimeout):
			// The worker is idle so stop the goroutine. It'll get
			// restarted automatically the next time we have an event to
			// send.
			log.Tracef("Queue %q has been idle for %s, going to sleep", oq.destination, queueIdleTimeout)
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
			for i := range pendingPDUs {
				pendingPDUs[i] = nil
			}
			for i := range pendingEDUs {
				pendingEDUs[i] = nil
			}
			pendingPDUs = nil
			pendingEDUs = nil
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

		// If we have pending PDUs or EDUs then construct a transaction.
		// Try sending the next transaction and see what happens.
		transaction, terr := oq.nextTransaction(pendingPDUs, pendingEDUs)
		if terr != nil {
			// We failed to send the transaction. Mark it as a failure.
			oq.statistics.Failure()
		} else if transaction {
			// If we successfully sent the transaction then clear out
			// the pending events and EDUs, and wipe our transaction ID.
			oq.statistics.Success()
			for i := range pendingPDUs {
				pendingPDUs[i] = nil
			}
			for i := range pendingEDUs {
				pendingEDUs[i] = nil
			}
			pendingPDUs = pendingPDUs[:0]
			pendingEDUs = pendingEDUs[:0]
		}
	}
}

// nextTransaction creates a new transaction from the pending event
// queue and sends it. Returns true if a transaction was sent or
// false otherwise.
// nolint:gocyclo
func (oq *destinationQueue) nextTransaction(
	pdus []*queuedPDU,
	edus []*queuedEDU,
) (bool, error) {
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

	// If we didn't get anything from the database and there are no
	// pending EDUs then there's nothing to do - stop here.
	if len(pdus) == 0 && len(edus) == 0 {
		return false, nil
	}

	// Pick out the transaction ID from the database. If we didn't
	// get a transaction ID (i.e. because there are no PDUs but only
	// EDUs) then generate a transaction ID.
	if t.TransactionID == "" {
		now := gomatrixserverlib.AsTimestamp(time.Now())
		t.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, oq.statistics.SuccessCount()))
	}

	var pduReceipts []*shared.Receipt
	var eduReceipts []*shared.Receipt

	// Go through PDUs that we retrieved from the database, if any,
	// and add them into the transaction.
	for _, pdu := range pdus {
		// Append the JSON of the event, since this is a json.RawMessage type in the
		// gomatrixserverlib.Transaction struct
		t.PDUs = append(t.PDUs, pdu.pdu.JSON())
		pduReceipts = append(pduReceipts, pdu.receipt)
	}

	// Do the same for pending EDUS in the queue.
	for _, edu := range edus {
		t.EDUs = append(t.EDUs, *edu.edu)
		eduReceipts = append(pduReceipts, edu.receipt)
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
		if pduReceipts != nil {
			//logrus.Infof("Cleaning PDUs %q", pduReceipt.String())
			if err = oq.db.CleanPDUs(context.Background(), oq.destination, pduReceipts); err != nil {
				log.WithError(err).Errorf("failed to clean PDUs for server %q", t.Destination)
			}
		}
		if eduReceipts != nil {
			//logrus.Infof("Cleaning EDUs %q", eduReceipt.String())
			if err = oq.db.CleanEDUs(context.Background(), oq.destination, eduReceipts); err != nil {
				log.WithError(err).Errorf("failed to clean EDUs for server %q", t.Destination)
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
