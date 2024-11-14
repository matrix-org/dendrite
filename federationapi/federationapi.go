// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package federationapi

import (
	"time"

	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/sirupsen/logrus"

	federationAPI "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/federationapi/consumers"
	"github.com/element-hq/dendrite/federationapi/internal"
	"github.com/element-hq/dendrite/federationapi/producers"
	"github.com/element-hq/dendrite/federationapi/queue"
	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/federationapi/storage"
	"github.com/element-hq/dendrite/internal/caching"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/jetstream"
	userapi "github.com/element-hq/dendrite/userapi/api"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/element-hq/dendrite/federationapi/routing"
)

// AddPublicRoutes sets up and registers HTTP handlers on the base API muxes for the FederationAPI component.
func AddPublicRoutes(
	processContext *process.ProcessContext,
	routers httputil.Routers,
	dendriteConfig *config.Dendrite,
	natsInstance *jetstream.NATSInstance,
	userAPI userapi.FederationUserAPI,
	federation fclient.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier,
	rsAPI roomserverAPI.FederationRoomserverAPI,
	fedAPI federationAPI.FederationInternalAPI,
	enableMetrics bool,
) {
	cfg := &dendriteConfig.FederationAPI
	mscCfg := &dendriteConfig.MSCs
	js, _ := natsInstance.Prepare(processContext, &cfg.Matrix.JetStream)
	producer := &producers.SyncAPIProducer{
		JetStream:              js,
		TopicReceiptEvent:      cfg.Matrix.JetStream.Prefixed(jetstream.OutputReceiptEvent),
		TopicSendToDeviceEvent: cfg.Matrix.JetStream.Prefixed(jetstream.OutputSendToDeviceEvent),
		TopicTypingEvent:       cfg.Matrix.JetStream.Prefixed(jetstream.OutputTypingEvent),
		TopicPresenceEvent:     cfg.Matrix.JetStream.Prefixed(jetstream.OutputPresenceEvent),
		TopicDeviceListUpdate:  cfg.Matrix.JetStream.Prefixed(jetstream.InputDeviceListUpdate),
		TopicSigningKeyUpdate:  cfg.Matrix.JetStream.Prefixed(jetstream.InputSigningKeyUpdate),
		Config:                 cfg,
		UserAPI:                userAPI,
	}

	// the federationapi component is a bit unique in that it attaches public routes AND serves
	// internal APIs (because it used to be 2 components: the 2nd being fedsender). As a result,
	// the constructor shape is a bit wonky in that it is not valid to AddPublicRoutes without a
	// concrete impl of FederationInternalAPI as the public routes and the internal API _should_
	// be the same thing now.
	f, ok := fedAPI.(*internal.FederationInternalAPI)
	if !ok {
		panic("federationapi.AddPublicRoutes called with a FederationInternalAPI impl which was not " +
			"FederationInternalAPI. This is a programming error.")
	}

	routing.Setup(
		routers,
		dendriteConfig,
		rsAPI, f, keyRing,
		federation, userAPI, mscCfg,
		producer, enableMetrics,
	)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	processContext *process.ProcessContext,
	dendriteCfg *config.Dendrite,
	cm *sqlutil.Connections,
	natsInstance *jetstream.NATSInstance,
	federation fclient.FederationClient,
	rsAPI roomserverAPI.FederationRoomserverAPI,
	caches *caching.Caches,
	keyRing *gomatrixserverlib.KeyRing,
	resetBlacklist bool,
) *internal.FederationInternalAPI {
	cfg := &dendriteCfg.FederationAPI

	federationDB, err := storage.NewDatabase(processContext.Context(), cm, &cfg.Database, caches, dendriteCfg.Global.IsLocalServerName)
	if err != nil {
		logrus.WithError(err).Panic("failed to connect to federation sender db")
	}

	if resetBlacklist {
		_ = federationDB.RemoveAllServersFromBlacklist()
	}

	stats := statistics.NewStatistics(federationDB, cfg.FederationMaxRetries+1, cfg.P2PFederationRetriesUntilAssumedOffline+1, cfg.EnableRelays)

	js, nats := natsInstance.Prepare(processContext, &cfg.Matrix.JetStream)

	signingInfo := dendriteCfg.Global.SigningIdentities()

	queues := queue.NewOutgoingQueues(
		federationDB, processContext,
		cfg.Matrix.DisableFederation,
		cfg.Matrix.ServerName, federation, &stats,
		signingInfo,
	)

	rsConsumer := consumers.NewOutputRoomEventConsumer(
		processContext, cfg, js, nats, queues,
		federationDB, rsAPI,
	)
	if err = rsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start room server consumer")
	}
	tsConsumer := consumers.NewOutputSendToDeviceConsumer(
		processContext, cfg, js, queues, federationDB,
	)
	if err = tsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start send-to-device consumer")
	}
	receiptConsumer := consumers.NewOutputReceiptConsumer(
		processContext, cfg, js, queues, federationDB,
	)
	if err = receiptConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start receipt consumer")
	}
	typingConsumer := consumers.NewOutputTypingConsumer(
		processContext, cfg, js, queues, federationDB,
	)
	if err = typingConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start typing consumer")
	}
	keyConsumer := consumers.NewKeyChangeConsumer(
		processContext, &dendriteCfg.KeyServer, js, queues, federationDB, rsAPI,
	)
	if err = keyConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start key server consumer")
	}

	presenceConsumer := consumers.NewOutputPresenceConsumer(
		processContext, cfg, js, queues, federationDB, rsAPI,
	)
	if err = presenceConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start presence consumer")
	}

	var cleanExpiredEDUs func()
	cleanExpiredEDUs = func() {
		logrus.Infof("Cleaning expired EDUs")
		if err := federationDB.DeleteExpiredEDUs(processContext.Context()); err != nil {
			logrus.WithError(err).Error("Failed to clean expired EDUs")
		}
		time.AfterFunc(time.Hour, cleanExpiredEDUs)
	}
	time.AfterFunc(time.Minute, cleanExpiredEDUs)

	return internal.NewFederationInternalAPI(federationDB, cfg, rsAPI, federation, &stats, caches, queues, keyRing)
}
