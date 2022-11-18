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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	fedapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/process"
)

// OutgoingQueues is a collection of queues for sending transactions to other
// matrix servers
type OutgoingQueues struct {
	db          storage.Database
	process     *process.ProcessContext
	disabled    bool
	rsAPI       api.FederationRoomserverAPI
	origin      gomatrixserverlib.ServerName
	client      fedapi.FederationClient
	statistics  *statistics.Statistics
	signing     map[gomatrixserverlib.ServerName]*gomatrixserverlib.SigningIdentity
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
		Subsystem: "federationapi",
		Name:      "destination_queues_total",
	},
)

var destinationQueueRunning = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "federationapi",
		Name:      "destination_queues_running",
	},
)

var destinationQueueBackingOff = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "federationapi",
		Name:      "destination_queues_backing_off",
	},
)

// NewOutgoingQueues makes a new OutgoingQueues
func NewOutgoingQueues(
	db storage.Database,
	process *process.ProcessContext,
	disabled bool,
	origin gomatrixserverlib.ServerName,
	client fedapi.FederationClient,
	rsAPI api.FederationRoomserverAPI,
	statistics *statistics.Statistics,
	signing []*gomatrixserverlib.SigningIdentity,
) *OutgoingQueues {
	queues := &OutgoingQueues{
		disabled:   disabled,
		process:    process,
		db:         db,
		rsAPI:      rsAPI,
		origin:     origin,
		client:     client,
		statistics: statistics,
		signing:    map[gomatrixserverlib.ServerName]*gomatrixserverlib.SigningIdentity{},
		queues:     map[gomatrixserverlib.ServerName]*destinationQueue{},
	}
	for _, identity := range signing {
		queues.signing[identity.ServerName] = identity
	}
	// Look up which servers we have pending items for and then rehydrate those queues.
	if !disabled {
		serverNames := map[gomatrixserverlib.ServerName]struct{}{}
		if names, err := db.GetPendingPDUServerNames(process.Context()); err == nil {
			for _, serverName := range names {
				serverNames[serverName] = struct{}{}
			}
		} else {
			log.WithError(err).Error("Failed to get PDU server names for destination queue hydration")
		}
		if names, err := db.GetPendingEDUServerNames(process.Context()); err == nil {
			for _, serverName := range names {
				serverNames[serverName] = struct{}{}
			}
		} else {
			log.WithError(err).Error("Failed to get EDU server names for destination queue hydration")
		}
		offset, step := time.Second*5, time.Second
		if max := len(serverNames); max > 120 {
			step = (time.Second * 120) / time.Duration(max)
		}
		for serverName := range serverNames {
			if queue := queues.getQueue(serverName); queue != nil {
				time.AfterFunc(offset, queue.wakeQueueIfNeeded)
				offset += step
			}
		}
	}
	return queues
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
	if !ok || oq == nil {
		destinationQueueTotal.Inc()
		oq = &destinationQueue{
			queues:      oqs,
			db:          oqs.db,
			process:     oqs.process,
			rsAPI:       oqs.rsAPI,
			origin:      oqs.origin,
			destination: destination,
			client:      oqs.client,
			statistics:  oqs.statistics.ForServer(destination),
			notify:      make(chan struct{}, 1),
			signing:     oqs.signing,
		}
		oq.statistics.AssignBackoffNotifier(oq.handleBackoffNotifier)
		oqs.queues[destination] = oq
	}
	return oq
}

// clearQueue removes the queue for the provided destination from the
// set of destination queues.
func (oqs *OutgoingQueues) clearQueue(oq *destinationQueue) {
	oqs.queuesMutex.Lock()
	defer oqs.queuesMutex.Unlock()

	delete(oqs.queues, oq.destination)
	destinationQueueTotal.Dec()
}

// SendEvent sends an event to the destinations
func (oqs *OutgoingQueues) SendEvent(
	ev *gomatrixserverlib.HeaderedEvent, origin gomatrixserverlib.ServerName,
	destinations []gomatrixserverlib.ServerName,
) error {
	if oqs.disabled {
		log.Trace("Federation is disabled, not sending event")
		return nil
	}
	if _, ok := oqs.signing[origin]; !ok {
		return fmt.Errorf(
			"sendevent: unexpected server to send as %q",
			origin,
		)
	}

	// Deduplicate destinations and remove the origin from the list of
	// destinations just to be sure.
	destmap := map[gomatrixserverlib.ServerName]struct{}{}
	for _, d := range destinations {
		destmap[d] = struct{}{}
	}
	delete(destmap, oqs.origin)
	for local := range oqs.signing {
		delete(destmap, local)
	}

	// Check if any of the destinations are prohibited by server ACLs.
	for destination := range destmap {
		if api.IsServerBannedFromRoom(
			oqs.process.Context(),
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

	nid, err := oqs.db.StoreJSON(oqs.process.Context(), string(headeredJSON))
	if err != nil {
		return fmt.Errorf("sendevent: oqs.db.StoreJSON: %w", err)
	}

	destQueues := make([]*destinationQueue, 0, len(destmap))
	for destination := range destmap {
		if queue := oqs.getQueue(destination); queue != nil {
			destQueues = append(destQueues, queue)
		} else {
			delete(destmap, destination)
		}
	}

	// Create a database entry that associates the given PDU NID with
	// this destinations queue. We'll then be able to retrieve the PDU
	// later.
	if err := oqs.db.AssociatePDUWithDestinations(
		oqs.process.Context(),
		destmap,
		nid, // NIDs from federationapi_queue_json table
	); err != nil {
		logrus.WithError(err).Errorf("failed to associate PDUs %q with destinations", nid)
		return err
	}

	// NOTE : PDUs should be associated with destinations before sending
	// them, otherwise this is technically a race.
	// If the send completes before they are associated then they won't
	// get properly cleaned up in the database.
	for _, queue := range destQueues {
		queue.sendEvent(ev, nid)
	}

	return nil
}

// SendEDU sends an EDU event to the destinations.
func (oqs *OutgoingQueues) SendEDU(
	e *gomatrixserverlib.EDU, origin gomatrixserverlib.ServerName,
	destinations []gomatrixserverlib.ServerName,
) error {
	if oqs.disabled {
		log.Trace("Federation is disabled, not sending EDU")
		return nil
	}
	if _, ok := oqs.signing[origin]; !ok {
		return fmt.Errorf(
			"sendevent: unexpected server to send as %q",
			origin,
		)
	}

	// Deduplicate destinations and remove the origin from the list of
	// destinations just to be sure.
	destmap := map[gomatrixserverlib.ServerName]struct{}{}
	for _, d := range destinations {
		destmap[d] = struct{}{}
	}
	delete(destmap, oqs.origin)
	for local := range oqs.signing {
		delete(destmap, local)
	}

	// There is absolutely no guarantee that the EDU will have a room_id
	// field, as it is not required by the spec. However, if it *does*
	// (e.g. typing notifications) then we should try to make sure we don't
	// bother sending them to servers that are prohibited by the server
	// ACLs.
	if result := gjson.GetBytes(e.Content, "room_id"); result.Exists() {
		for destination := range destmap {
			if api.IsServerBannedFromRoom(
				oqs.process.Context(),
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
		sentry.CaptureException(err)
		return fmt.Errorf("json.Marshal: %w", err)
	}

	nid, err := oqs.db.StoreJSON(oqs.process.Context(), string(ephemeralJSON))
	if err != nil {
		sentry.CaptureException(err)
		return fmt.Errorf("sendevent: oqs.db.StoreJSON: %w", err)
	}

	destQueues := make([]*destinationQueue, 0, len(destmap))
	for destination := range destmap {
		if queue := oqs.getQueue(destination); queue != nil {
			destQueues = append(destQueues, queue)
		} else {
			delete(destmap, destination)
		}
	}

	// Create a database entry that associates the given PDU NID with
	// these destination queues. We'll then be able to retrieve the PDU
	// later.
	if err := oqs.db.AssociateEDUWithDestinations(
		oqs.process.Context(),
		destmap, // the destination server names
		nid,     // NIDs from federationapi_queue_json table
		e.Type,
		nil, // this will use the default expireEDUTypes map
	); err != nil {
		logrus.WithError(err).Errorf("failed to associate EDU with destinations")
		return err
	}

	// NOTE : EDUs should be associated with destinations before sending
	// them, otherwise this is technically a race.
	// If the send completes before they are associated then they won't
	// get properly cleaned up in the database.
	for _, queue := range destQueues {
		queue.sendEDU(e, nid)
	}

	return nil
}

// IsServerBlacklisted returns whether or not the provided server is currently
// blacklisted.
func (oqs *OutgoingQueues) IsServerBlacklisted(srv gomatrixserverlib.ServerName) bool {
	return oqs.statistics.ForServer(srv).Blacklisted()
}

// RetryServer attempts to resend events to the given server if we had given up.
func (oqs *OutgoingQueues) RetryServer(srv gomatrixserverlib.ServerName) {
	if oqs.disabled {
		return
	}

	serverStatistics := oqs.statistics.ForServer(srv)
	forceWakeup := serverStatistics.Blacklisted()
	serverStatistics.RemoveBlacklist()
	serverStatistics.ClearBackoff()

	if queue := oqs.getQueue(srv); queue != nil {
		queue.wakeQueueIfEventsPending(forceWakeup)
	}
}
