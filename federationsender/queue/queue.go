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
	"time"

	"github.com/matrix-org/dendrite/federationsender/statistics"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// OutgoingQueues is a collection of queues for sending transactions to other
// matrix servers
type OutgoingQueues struct {
	db          storage.Database
	process     *process.ProcessContext
	disabled    bool
	rsAPI       api.RoomserverInternalAPI
	origin      gomatrixserverlib.ServerName
	client      *gomatrixserverlib.FederationClient
	statistics  *statistics.Statistics
	signing     *SigningInfo
	queuesMutex sync.Mutex // protects the below
	queues      map[gomatrixserverlib.ServerName]*destinationQueue
}

func init() {
	prometheus.MustRegister(
		destinationQueueTotal, destinationQueueRunning,
		destinationQueueBackingOff,
	)
}

var destinationQueueTotal = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "federationsender",
		Name:      "destination_queues_total",
	},
)

var destinationQueueRunning = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "federationsender",
		Name:      "destination_queues_running",
	},
)

var destinationQueueBackingOff = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "federationsender",
		Name:      "destination_queues_backing_off",
	},
)

// NewOutgoingQueues makes a new OutgoingQueues
func NewOutgoingQueues(
	db storage.Database,
	process *process.ProcessContext,
	disabled bool,
	origin gomatrixserverlib.ServerName,
	client *gomatrixserverlib.FederationClient,
	rsAPI api.RoomserverInternalAPI,
	statistics *statistics.Statistics,
	signing *SigningInfo,
) *OutgoingQueues {
	queues := &OutgoingQueues{
		disabled:   disabled,
		process:    process,
		db:         db,
		rsAPI:      rsAPI,
		origin:     origin,
		client:     client,
		statistics: statistics,
		signing:    signing,
		queues:     map[gomatrixserverlib.ServerName]*destinationQueue{},
	}
	// Look up which servers we have pending items for and then rehydrate those queues.
	if !disabled {
		time.AfterFunc(time.Second*5, func() {
			serverNames := map[gomatrixserverlib.ServerName]struct{}{}
			if names, err := db.GetPendingPDUServerNames(context.Background()); err == nil {
				for _, serverName := range names {
					serverNames[serverName] = struct{}{}
				}
			} else {
				log.WithError(err).Error("Failed to get PDU server names for destination queue hydration")
			}
			if names, err := db.GetPendingEDUServerNames(context.Background()); err == nil {
				for _, serverName := range names {
					serverNames[serverName] = struct{}{}
				}
			} else {
				log.WithError(err).Error("Failed to get EDU server names for destination queue hydration")
			}
			for serverName := range serverNames {
				if queue := queues.getQueue(serverName); queue != nil {
					queue.wakeQueueIfNeeded()
				}
			}
		})
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

type queuedPDU struct {
	receipt *shared.Receipt
	pdu     *gomatrixserverlib.HeaderedEvent
}

type queuedEDU struct {
	receipt *shared.Receipt
	edu     *gomatrixserverlib.EDU
}

func (oqs *OutgoingQueues) getQueue(destination gomatrixserverlib.ServerName) *destinationQueue {
	if oqs.statistics.ForServer(destination).Blacklisted() {
		return nil
	}
	oqs.queuesMutex.Lock()
	defer oqs.queuesMutex.Unlock()
	oq, ok := oqs.queues[destination]
	if !ok || oq != nil {
		destinationQueueTotal.Inc()
		oq = &destinationQueue{
			queues:           oqs,
			db:               oqs.db,
			process:          oqs.process,
			rsAPI:            oqs.rsAPI,
			origin:           oqs.origin,
			destination:      destination,
			client:           oqs.client,
			statistics:       oqs.statistics.ForServer(destination),
			notify:           make(chan struct{}, 1),
			interruptBackoff: make(chan bool),
			signing:          oqs.signing,
		}
		oqs.queues[destination] = oq
	}
	return oq
}

func (oqs *OutgoingQueues) clearQueue(oq *destinationQueue) {
	oqs.queuesMutex.Lock()
	defer oqs.queuesMutex.Unlock()

	delete(oqs.queues, oq.destination)
	destinationQueueTotal.Dec()
}

type ErrorFederationDisabled struct {
	Message string
}

func (e *ErrorFederationDisabled) Error() string {
	return e.Message
}

// SendEvent sends an event to the destinations
func (oqs *OutgoingQueues) SendEvent(
	ev *gomatrixserverlib.HeaderedEvent, origin gomatrixserverlib.ServerName,
	destinations []gomatrixserverlib.ServerName,
) error {
	if oqs.disabled {
		return &ErrorFederationDisabled{
			Message: "Federation disabled",
		}
	}
	if origin != oqs.origin {
		// TODO: Support virtual hosting; gh issue #577.
		return fmt.Errorf(
			"sendevent: unexpected server to send as: got %q expected %q",
			origin, oqs.origin,
		)
	}

	// Deduplicate destinations and remove the origin from the list of
	// destinations just to be sure.
	destmap := map[gomatrixserverlib.ServerName]struct{}{}
	for _, d := range destinations {
		destmap[d] = struct{}{}
	}
	delete(destmap, oqs.origin)

	// Check if any of the destinations are prohibited by server ACLs.
	for destination := range destmap {
		if api.IsServerBannedFromRoom(
			context.TODO(),
			oqs.rsAPI,
			ev.RoomID(),
			destination,
		) {
			delete(destmap, destination)
		}
	}

	// If there are no remaining destinations then give up.
	if len(destmap) == 0 {
		return nil
	}

	log.WithFields(log.Fields{
		"destinations": len(destmap), "event": ev.EventID(),
	}).Infof("Sending event")

	headeredJSON, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	nid, err := oqs.db.StoreJSON(context.TODO(), string(headeredJSON))
	if err != nil {
		return fmt.Errorf("sendevent: oqs.db.StoreJSON: %w", err)
	}

	for destination := range destmap {
		if queue := oqs.getQueue(destination); queue != nil {
			queue.sendEvent(ev, nid)
		}
	}

	return nil
}

// SendEDU sends an EDU event to the destinations.
func (oqs *OutgoingQueues) SendEDU(
	e *gomatrixserverlib.EDU, origin gomatrixserverlib.ServerName,
	destinations []gomatrixserverlib.ServerName,
) error {
	if oqs.disabled {
		return &ErrorFederationDisabled{
			Message: "Federation disabled",
		}
	}
	if origin != oqs.origin {
		// TODO: Support virtual hosting; gh issue #577.
		return fmt.Errorf(
			"sendevent: unexpected server to send as: got %q expected %q",
			origin, oqs.origin,
		)
	}

	// Deduplicate destinations and remove the origin from the list of
	// destinations just to be sure.
	destmap := map[gomatrixserverlib.ServerName]struct{}{}
	for _, d := range destinations {
		destmap[d] = struct{}{}
	}
	delete(destmap, oqs.origin)

	// There is absolutely no guarantee that the EDU will have a room_id
	// field, as it is not required by the spec. However, if it *does*
	// (e.g. typing notifications) then we should try to make sure we don't
	// bother sending them to servers that are prohibited by the server
	// ACLs.
	if result := gjson.GetBytes(e.Content, "room_id"); result.Exists() {
		for destination := range destmap {
			if api.IsServerBannedFromRoom(
				context.TODO(),
				oqs.rsAPI,
				result.Str,
				destination,
			) {
				delete(destmap, destination)
			}
		}
	}

	// If there are no remaining destinations then give up.
	if len(destmap) == 0 {
		return nil
	}

	log.WithFields(log.Fields{
		"destinations": len(destmap), "edu_type": e.Type,
	}).Info("Sending EDU event")

	ephemeralJSON, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	nid, err := oqs.db.StoreJSON(context.TODO(), string(ephemeralJSON))
	if err != nil {
		return fmt.Errorf("sendevent: oqs.db.StoreJSON: %w", err)
	}

	for destination := range destmap {
		if queue := oqs.getQueue(destination); queue != nil {
			queue.sendEDU(e, nid)
		}
	}

	return nil
}

// RetryServer attempts to resend events to the given server if we had given up.
func (oqs *OutgoingQueues) RetryServer(srv gomatrixserverlib.ServerName) {
	if oqs.disabled {
		return
	}
	if queue := oqs.getQueue(srv); queue != nil {
		queue.wakeQueueIfNeeded()
	}
}
