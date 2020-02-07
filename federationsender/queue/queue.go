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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// OutgoingQueues is a collection of queues for sending transactions to other
// matrix servers
type OutgoingQueues struct {
	db          storage.Database
	origin      gomatrixserverlib.ServerName
	client      *gomatrixserverlib.FederationClient
	queuesMutex sync.Mutex
	queues      map[gomatrixserverlib.ServerName]*destinationQueue // protected by queuesMutex
}

// NewOutgoingQueues makes a new OutgoingQueues
func NewOutgoingQueues(
	db storage.Database,
	origin gomatrixserverlib.ServerName,
	client *gomatrixserverlib.FederationClient,
) *OutgoingQueues {
	queues := OutgoingQueues{
		db:     db,
		origin: origin,
		client: client,
		queues: map[gomatrixserverlib.ServerName]*destinationQueue{},
	}

	go queues.processRetries()
	return &queues
}

func (oqs *OutgoingQueues) QueueEvent(
	destination gomatrixserverlib.ServerName,
	event gomatrixserverlib.Event,
	retryAt time.Time,
) error {
	if time.Until(retryAt) < time.Second*5 {
		return errors.New("can't queue for less than 5 seconds")
	}

	return oqs.db.QueueEventForRetry(
		context.Background(), // context
		string(oqs.origin),   // origin servername
		string(destination),  // destination servername
		event,                // event
		0,                    // attempts
		retryAt,              // retry at time
	)
}

func (oqs *OutgoingQueues) RemoveQueue(name gomatrixserverlib.ServerName) {
	oqs.queuesMutex.Lock()
	defer oqs.queuesMutex.Unlock()
	delete(oqs.queues, name)
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
		oq, ok := oqs.queues[destination]
		if !ok {
			oq = &destinationQueue{
				parent:      oqs,
				origin:      oqs.origin,
				destination: destination,
				client:      oqs.client,
			}
			oqs.queues[destination] = oq
		}

		oq.sendEvent(&types.PendingPDU{
			PDU: ev,
		})
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

		oq.sendEDU(&types.PendingEDU{
			EDU: e,
		})
	}

	return nil
}

func (oqs *OutgoingQueues) processRetries() {
	ctx := context.Background()
	for {
		time.Sleep(time.Second * 5)
		fmt.Println("trying to process retries")

		retries, err := oqs.db.SelectRetryEventsPending(ctx)
		if err != nil {
			fmt.Println("failed:", err)
			continue
		}

		fmt.Println("there are", len(retries), "PDUs to retry sending")

		for _, retry := range retries {
			fmt.Println("retrying:", retry)
		}

		oqs.db.DeleteRetryExpiredEvents(ctx)
	}
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
