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
	"fmt"
	"sync"

	"github.com/matrix-org/dendrite/federationsender/producers"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// OutgoingQueues is a collection of queues for sending transactions to other
// matrix servers
type OutgoingQueues struct {
	rsProducer *producers.RoomserverProducer
	origin     gomatrixserverlib.ServerName
	client     *gomatrixserverlib.FederationClient
	// The queuesMutex protects queues
	queuesMutex sync.RWMutex
	queues      map[gomatrixserverlib.ServerName]*destinationQueue
}

// NewOutgoingQueues makes a new OutgoingQueues
func NewOutgoingQueues(
	origin gomatrixserverlib.ServerName,
	client *gomatrixserverlib.FederationClient,
	rsProducer *producers.RoomserverProducer,
) *OutgoingQueues {
	return &OutgoingQueues{
		rsProducer: rsProducer,
		origin:     origin,
		client:     client,
		queues:     map[gomatrixserverlib.ServerName]*destinationQueue{},
	}
}

// SendEvent sends an event to the destinations
func (oqs *OutgoingQueues) SendEvent(
	ev *gomatrixserverlib.HeaderedEvent, origin gomatrixserverlib.ServerName,
	destinations []gomatrixserverlib.ServerName,
) error {
	if origin != oqs.origin {
		// TODO: Support virtual hosting; gh issue #577.
		return fmt.Errorf(
			"sendevent: unexpected server to send as: got %q expected %q",
			origin, oqs.origin,
		)
	}

	// Remove our own server from the list of destinations.
	destinations = filterDestinations(oqs.origin, destinations)

	log.WithFields(log.Fields{
		"destinations": destinations, "event": ev.EventID(),
	}).Info("Sending event")

	for _, destination := range destinations {
		oqs.queuesMutex.RLock()
		oq := oqs.queues[destination]
		oqs.queuesMutex.RUnlock()
		if oq == nil {
			oq = &destinationQueue{
				rsProducer:  oqs.rsProducer,
				origin:      oqs.origin,
				destination: destination,
				client:      oqs.client,
			}
			oqs.queuesMutex.Lock()
			oqs.queues[destination] = oq
			oqs.queuesMutex.Unlock()
		}

		go oq.sendEvent(ev)
	}

	return nil
}

// SendEvent sends an event to the destinations
func (oqs *OutgoingQueues) SendInvite(
	inviteReq *gomatrixserverlib.InviteV2Request,
) error {
	ev := inviteReq.Event()
	stateKey := ev.StateKey()
	if stateKey == nil {
		log.WithFields(log.Fields{
			"event_id": ev.EventID(),
		}).Info("invite had no state key, dropping")
		return nil
	}

	_, destination, err := gomatrixserverlib.SplitID('@', *stateKey)
	if err != nil {
		log.WithFields(log.Fields{
			"event_id":  ev.EventID(),
			"state_key": stateKey,
		}).Info("failed to split destination from state key")
		return nil
	}

	log.WithFields(log.Fields{
		"event_id": ev.EventID(),
	}).Info("Sending invite")

	oqs.queuesMutex.RLock()
	oq := oqs.queues[destination]
	oqs.queuesMutex.RUnlock()
	if oq == nil {
		oq = &destinationQueue{
			rsProducer:  oqs.rsProducer,
			origin:      oqs.origin,
			destination: destination,
			client:      oqs.client,
		}
		oqs.queuesMutex.Lock()
		oqs.queues[destination] = oq
		oqs.queuesMutex.Unlock()
	}

	go oq.sendInvite(inviteReq)

	return nil
}

// SendEDU sends an EDU event to the destinations
func (oqs *OutgoingQueues) SendEDU(
	e *gomatrixserverlib.EDU, origin gomatrixserverlib.ServerName,
	destinations []gomatrixserverlib.ServerName,
) error {
	if origin != oqs.origin {
		// TODO: Support virtual hosting; gh issue #577.
		return fmt.Errorf(
			"sendevent: unexpected server to send as: got %q expected %q",
			origin, oqs.origin,
		)
	}

	// Remove our own server from the list of destinations.
	destinations = filterDestinations(oqs.origin, destinations)

	if len(destinations) > 0 {
		log.WithFields(log.Fields{
			"destinations": destinations, "edu_type": e.Type,
		}).Info("Sending EDU event")
	}

	for _, destination := range destinations {
		oqs.queuesMutex.RLock()
		oq := oqs.queues[destination]
		oqs.queuesMutex.RUnlock()
		if oq == nil {
			oq = &destinationQueue{
				rsProducer:  oqs.rsProducer,
				origin:      oqs.origin,
				destination: destination,
				client:      oqs.client,
			}
			oqs.queuesMutex.Lock()
			oqs.queues[destination] = oq
			oqs.queuesMutex.Unlock()
		}

		go oq.sendEDU(e)
	}

	return nil
}

// filterDestinations removes our own server from the list of destinations.
// Otherwise we could end up trying to talk to ourselves.
func filterDestinations(origin gomatrixserverlib.ServerName, destinations []gomatrixserverlib.ServerName) []gomatrixserverlib.ServerName {
	var result []gomatrixserverlib.ServerName
	for _, destination := range destinations {
		if destination == origin {
			continue
		}
		result = append(result, destination)
	}
	return result
}
