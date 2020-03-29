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

	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// OutgoingQueues is a collection of queues for sending transactions to other
// matrix servers
type OutgoingQueues struct {
	origin gomatrixserverlib.ServerName
	client *gomatrixserverlib.FederationClient
	// The queuesMutex protects queues
	queuesMutex sync.Mutex
	queues      map[gomatrixserverlib.ServerName]*destinationQueue
}

// NewOutgoingQueues makes a new OutgoingQueues
func NewOutgoingQueues(origin gomatrixserverlib.ServerName, client *gomatrixserverlib.FederationClient) *OutgoingQueues {
	return &OutgoingQueues{
		origin: origin,
		client: client,
		queues: map[gomatrixserverlib.ServerName]*destinationQueue{},
	}
}

// SendEvent sends an event to the destinations
func (oqs *OutgoingQueues) SendEvent(
	ev *gomatrixserverlib.Event, origin gomatrixserverlib.ServerName,
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

	oqs.queuesMutex.Lock()
	defer oqs.queuesMutex.Unlock()
	for _, destination := range destinations {
		oq := oqs.queues[destination]
		if oq == nil {
			oq = &destinationQueue{
				origin:      oqs.origin,
				destination: destination,
				client:      oqs.client,
			}
			oqs.queues[destination] = oq
		}

		oq.sendEvent(ev)
	}

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

	oqs.queuesMutex.Lock()
	defer oqs.queuesMutex.Unlock()
	for _, destination := range destinations {
		oq := oqs.queues[destination]
		if oq == nil {
			oq = &destinationQueue{
				origin:      oqs.origin,
				destination: destination,
				client:      oqs.client,
			}
			oqs.queues[destination] = oq
		}

		oq.sendEDU(e)
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
