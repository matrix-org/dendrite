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
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// OutgoingQueues is a collection of queues for sending transactions to other
// matrix servers
type OutgoingQueues struct {
	db          storage.Database
	rsAPI       api.RoomserverInternalAPI
	origin      gomatrixserverlib.ServerName
	client      *gomatrixserverlib.FederationClient
	statistics  *types.Statistics
	signing     *SigningInfo
	queuesMutex sync.Mutex // protects the below
	queues      map[gomatrixserverlib.ServerName]*destinationQueue
}

// NewOutgoingQueues makes a new OutgoingQueues
func NewOutgoingQueues(
	db storage.Database,
	origin gomatrixserverlib.ServerName,
	client *gomatrixserverlib.FederationClient,
	rsAPI api.RoomserverInternalAPI,
	statistics *types.Statistics,
	signing *SigningInfo,
) *OutgoingQueues {
	queues := &OutgoingQueues{
		db:         db,
		rsAPI:      rsAPI,
		origin:     origin,
		client:     client,
		statistics: statistics,
		signing:    signing,
		queues:     map[gomatrixserverlib.ServerName]*destinationQueue{},
	}
	// Look up which servers we have pending items for and then rehydrate those queues.
	if serverNames, err := db.GetPendingServerNames(context.Background()); err == nil {
		for _, serverName := range serverNames {
			queues.getQueue(serverName).wakeQueueIfNeeded()
		}
	} else {
		log.WithError(err).Error("Failed to get server names for destination queue hydration")
	}
	return queues
}

// TODO: Move this somewhere useful for other components as we often need to ferry these 3 variables
// around together
type SigningInfo struct {
	ServerName gomatrixserverlib.ServerName
	KeyID      gomatrixserverlib.KeyID
	PrivateKey ed25519.PrivateKey
}

func (oqs *OutgoingQueues) getQueue(destination gomatrixserverlib.ServerName) *destinationQueue {
	oqs.queuesMutex.Lock()
	defer oqs.queuesMutex.Unlock()
	oq := oqs.queues[destination]
	if oq == nil {
		oq = &destinationQueue{
			db:               oqs.db,
			rsAPI:            oqs.rsAPI,
			origin:           oqs.origin,
			destination:      destination,
			client:           oqs.client,
			statistics:       oqs.statistics.ForServer(destination),
			incomingEDUs:     make(chan *gomatrixserverlib.EDU, 128),
			incomingInvites:  make(chan *gomatrixserverlib.InviteV2Request, 128),
			notifyPDUs:       make(chan bool, 1),
			interruptBackoff: make(chan bool),
			signing:          oqs.signing,
		}
		oqs.queues[destination] = oq
	}
	return oq
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
	destinations = filterAndDedupeDests(oqs.origin, destinations)
	if len(destinations) == 0 {
		return nil
	}

	log.WithFields(log.Fields{
		"destinations": destinations, "event": ev.EventID(),
	}).Info("Sending event")

	headeredJSON, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	nid, err := oqs.db.StoreJSON(context.TODO(), string(headeredJSON))
	if err != nil {
		return fmt.Errorf("sendevent: oqs.db.StoreJSON: %w", err)
	}

	for _, destination := range destinations {
		oqs.getQueue(destination).sendEvent(nid)
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
		"event_id":    ev.EventID(),
		"server_name": destination,
	}).Info("Sending invite")

	oqs.getQueue(destination).sendInvite(inviteReq)

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
	destinations = filterAndDedupeDests(oqs.origin, destinations)

	if len(destinations) > 0 {
		log.WithFields(log.Fields{
			"destinations": destinations, "edu_type": e.Type,
		}).Info("Sending EDU event")
	}

	for _, destination := range destinations {
		oqs.getQueue(destination).sendEDU(e)
	}

	return nil
}

// RetryServer attempts to resend events to the given server if we had given up.
func (oqs *OutgoingQueues) RetryServer(srv gomatrixserverlib.ServerName) {
	q := oqs.getQueue(srv)
	if q == nil {
		return
	}
	q.wakeQueueIfNeeded()
}

// filterAndDedupeDests removes our own server from the list of destinations
// and deduplicates any servers in the list that may appear more than once.
func filterAndDedupeDests(origin gomatrixserverlib.ServerName, destinations []gomatrixserverlib.ServerName) (
	result []gomatrixserverlib.ServerName,
) {
	strs := make([]string, len(destinations))
	for i, d := range destinations {
		strs[i] = string(d)
	}
	for _, destination := range util.UniqueStrings(strs) {
		if gomatrixserverlib.ServerName(destination) == origin {
			continue
		}
		result = append(result, gomatrixserverlib.ServerName(destination))
	}
	return result
}
