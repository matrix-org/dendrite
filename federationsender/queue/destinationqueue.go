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
	statistics         *types.ServerStatistics                 // statistics about this remote server
	incomingPDUs       chan *gomatrixserverlib.HeaderedEvent   // PDUs to send
	incomingEDUs       chan *gomatrixserverlib.EDU             // EDUs to send
	incomingInvites    chan *gomatrixserverlib.InviteV2Request // invites to send
	lastTransactionIDs []gomatrixserverlib.TransactionID       // last transaction ID
	pendingPDUs        []*gomatrixserverlib.HeaderedEvent      // owned by backgroundSend
	pendingEDUs        []*gomatrixserverlib.EDU                // owned by backgroundSend
	pendingInvites     []*gomatrixserverlib.InviteV2Request    // owned by backgroundSend
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
	// Mark the worker as running for its lifetime.
	oq.running.Store(true)
	defer oq.running.Store(false)

	for {
		// Wait either for incoming events, or until we hit an
		// idle timeout.
		select {
		case pdu := <-oq.incomingPDUs:
			oq.pendingPDUs = append(oq.pendingPDUs, pdu)
		case edu := <-oq.incomingEDUs:
			oq.pendingEDUs = append(oq.pendingEDUs, edu)
		case invite := <-oq.incomingInvites:
			oq.pendingInvites = append(oq.pendingInvites, invite)
		case <-time.After(time.Second * 30):
			// The worker is idle so stop the goroutine. It'll
			// get restarted automatically the next time we
			// get an event.
			return
		}

		// If we are backing off this server then wait for the
		// backoff duration to complete first.
		if backoff, duration := oq.statistics.BackoffDuration(); backoff {
			<-time.After(duration)
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
