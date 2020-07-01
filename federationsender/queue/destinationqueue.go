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

	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

const maxPDUsPerTransaction = 50

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
	statistics         *types.ServerStatistics                 // statistics about this remote server
	incomingInvites    chan *gomatrixserverlib.InviteV2Request // invites to send
	incomingEDUs       chan *gomatrixserverlib.EDU             // EDUs to send
	transactionIDMutex sync.Mutex                              // protects transactionID
	transactionID      gomatrixserverlib.TransactionID         // last transaction ID
	transactionCount   atomic.Int32                            // how many events in this transaction so far
	pendingPDUs        atomic.Int32                            // how many PDUs are waiting to be sent
	pendingEDUs        []*gomatrixserverlib.EDU                // owned by backgroundSend
	pendingInvites     []*gomatrixserverlib.InviteV2Request    // owned by backgroundSend
	wakeServerCh       chan bool                               // interrupts idle wait
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
	//
	// Interrupt the backoff. If the federation request that happens as a result of this is successful
	// then the counters will be reset there and the backoff will cancel. If the federation request
	// fails then we will retry at the current backoff interval, so as to prevent us from spamming
	// homeservers which are behaving badly.
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
func (oq *destinationQueue) sendEvent(nid int64) {
	if oq.statistics.Blacklisted() {
		// If the destination is blacklisted then drop the event.
		return
	}
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
		[]int64{nid},     // NID from federationsender_queue_json table
	); err != nil {
		log.WithError(err).Errorf("failed to associate PDU NID %d with destination %q", nid, oq.destination)
		return
	}
	// We've successfully added a PDU to the transaction so increase
	// the counter.
	oq.transactionCount.Add(1)
	// If the queue isn't running at this point then start it.
	if !oq.running.Load() {
		go oq.backgroundSend()
	}
	// Signal that we've sent a new PDU. This will cause the queue to
	// wake up if it's asleep. The return to the Add function will only
	// be 1 if the previous value was 0, e.g. nothing was waiting before.
	if oq.pendingPDUs.Add(1) == 1 {
		oq.wakeServerCh <- true
	}
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
		// If we have nothing to do then wait either for incoming events, or
		// until we hit an idle timeout.
		if oq.pendingPDUs.Load() == 0 && len(oq.pendingEDUs) == 0 && len(oq.pendingInvites) == 0 {
			select {
			case <-oq.wakeServerCh:
				// We were woken up because there are new PDUs waiting in the
				// database.
			case edu := <-oq.incomingEDUs:
				// EDUs are handled in-memory for now. We will try to keep
				// the ordering intact.
				// TODO: Certain EDU types need persistence, e.g. send-to-device
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
				// The worker is idle so stop the goroutine. It'll get
				// restarted automatically the next time we have an event to
				// send.
				return
			}
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

		// If we have pending PDUs or EDUs then construct a transaction.
		if oq.pendingPDUs.Load() > 0 || len(oq.pendingEDUs) > 0 {
			// Try sending the next transaction and see what happens.
			transaction, terr := oq.nextTransaction(oq.pendingEDUs)
			if terr != nil {
				// We failed to send the transaction.
				if giveUp := oq.statistics.Failure(); giveUp {
					// It's been suggested that we should give up because the backoff
					// has exceeded a maximum allowable value. Clean up the in-memory
					// buffers at this point. The PDU clean-up is already on a defer.
					oq.cleanPendingEDUs()
					oq.cleanPendingInvites()
					return
				}
			} else if transaction {
				// If we successfully sent the transaction then clear out
				// the pending events and EDUs, and wipe our transaction ID.
				oq.statistics.Success()
				// Clean up the in-memory buffers.
				oq.cleanPendingEDUs()
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

// cleanPendingEDUs cleans out the pending EDU buffer, removing
// all references so that the underlying objects can be GC'd.
func (oq *destinationQueue) cleanPendingEDUs() {
	for i := 0; i < len(oq.pendingEDUs); i++ {
		oq.pendingEDUs[i] = nil
	}
	oq.pendingEDUs = []*gomatrixserverlib.EDU{}
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
func (oq *destinationQueue) nextTransaction(
	pendingEDUs []*gomatrixserverlib.EDU,
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

	// Ask the database for any pending PDUs from the next transaction.
	// maxPDUsPerTransaction is an upper limit but we probably won't
	// actually retrieve that many events.
	txid, pdus, err := oq.db.GetNextTransactionPDUs(
		context.TODO(),        // context
		oq.destination,        // server name
		maxPDUsPerTransaction, // max events to retrieve
	)
	if err != nil {
		log.WithError(err).Errorf("failed to get next transaction PDUs for server %q", oq.destination)
		return false, fmt.Errorf("oq.db.GetNextTransactionPDUs: %w", err)
	}

	// If we didn't get anything from the database and there are no
	// pending EDUs then there's nothing to do - stop here.
	if len(pdus) == 0 && len(pendingEDUs) == 0 {
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
	for _, edu := range pendingEDUs {
		t.EDUs = append(t.EDUs, *edu)
	}

	logrus.WithField("server_name", oq.destination).Infof("Sending transaction %q containing %d PDUs, %d EDUs", t.TransactionID, len(t.PDUs), len(t.EDUs))

	// Try to send the transaction to the destination server.
	// TODO: we should check for 500-ish fails vs 400-ish here,
	// since we shouldn't queue things indefinitely in response
	// to a 400-ish error
	_, err = oq.client.SendTransaction(context.TODO(), t)
	switch e := err.(type) {
	case nil:
		// No error was returned so the transaction looks to have
		// been successfully sent.
		oq.pendingPDUs.Sub(int32(len(t.PDUs)))
		// Clean up the transaction in the database.
		if err = oq.db.CleanTransactionPDUs(
			context.TODO(),
			t.Destination,
			t.TransactionID,
		); err != nil {
			log.WithError(err).Errorf("failed to clean transaction %q for server %q", t.TransactionID, t.Destination)
		}
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
