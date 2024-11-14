// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package queue

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/federationapi/storage"
	"github.com/element-hq/dendrite/federationapi/storage/shared/receipt"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/process"
)

// OutgoingQueues is a collection of queues for sending transactions to other
// matrix servers
type OutgoingQueues struct {
	db          storage.Database
	process     *process.ProcessContext
	disabled    bool
	origin      spec.ServerName
	client      fclient.FederationClient
	statistics  *statistics.Statistics
	signing     map[spec.ServerName]*fclient.SigningIdentity
	queuesMutex sync.Mutex // protects the below
	queues      map[spec.ServerName]*destinationQueue
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
	origin spec.ServerName,
	client fclient.FederationClient,
	statistics *statistics.Statistics,
	signing []*fclient.SigningIdentity,
) *OutgoingQueues {
	queues := &OutgoingQueues{
		disabled:   disabled,
		process:    process,
		db:         db,
		origin:     origin,
		client:     client,
		statistics: statistics,
		signing:    map[spec.ServerName]*fclient.SigningIdentity{},
		queues:     map[spec.ServerName]*destinationQueue{},
	}
	for _, identity := range signing {
		queues.signing[identity.ServerName] = identity
	}
	// Look up which servers we have pending items for and then rehydrate those queues.
	if !disabled {
		serverNames := map[spec.ServerName]struct{}{}
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
	dbReceipt *receipt.Receipt
	pdu       *types.HeaderedEvent
}

type queuedEDU struct {
	dbReceipt *receipt.Receipt
	edu       *gomatrixserverlib.EDU
}

func (oqs *OutgoingQueues) getQueue(destination spec.ServerName) *destinationQueue {
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
	ev *types.HeaderedEvent, origin spec.ServerName,
	destinations []spec.ServerName,
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
	destmap := map[spec.ServerName]struct{}{}
	for _, d := range destinations {
		destmap[d] = struct{}{}
	}
	delete(destmap, oqs.origin)
	for local := range oqs.signing {
		delete(destmap, local)
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
	e *gomatrixserverlib.EDU, origin spec.ServerName,
	destinations []spec.ServerName,
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
	destmap := map[spec.ServerName]struct{}{}
	for _, d := range destinations {
		destmap[d] = struct{}{}
	}
	delete(destmap, oqs.origin)
	for local := range oqs.signing {
		delete(destmap, local)
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

// RetryServer attempts to resend events to the given server if we had given up.
func (oqs *OutgoingQueues) RetryServer(srv spec.ServerName, wasBlacklisted bool) {
	if oqs.disabled {
		return
	}

	if queue := oqs.getQueue(srv); queue != nil {
		queue.wakeQueueIfEventsPending(wasBlacklisted)
	}
}
