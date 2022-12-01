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

package federationapi

import (
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/federationapi/api"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/consumers"
	"github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/inthttp"
	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/internal/caching"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/jetstream"
	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/federationapi/routing"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.FederationInternalAPI) {
	inthttp.AddRoutes(intAPI, router)
}

// AddPublicRoutes sets up and registers HTTP handlers on the base API muxes for the FederationAPI component.
func AddPublicRoutes(
	base *base.BaseDendrite,
	userAPI userapi.UserInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier,
	rsAPI roomserverAPI.FederationRoomserverAPI,
	fedAPI federationAPI.FederationInternalAPI,
	keyAPI keyserverAPI.FederationKeyAPI,
	servers federationAPI.ServersInRoomProvider,
) {
	cfg := &base.Cfg.FederationAPI
	mscCfg := &base.Cfg.MSCs
	js, _ := base.NATS.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)
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
		base.PublicFederationAPIMux,
		base.PublicKeyAPIMux,
		base.PublicWellKnownAPIMux,
		cfg,
		rsAPI, f, keyRing,
		federation, userAPI, keyAPI, mscCfg,
		servers, producer,
	)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite,
	federation api.FederationClient,
	rsAPI roomserverAPI.FederationRoomserverAPI,
	caches *caching.Caches,
	keyRing *gomatrixserverlib.KeyRing,
	resetBlacklist bool,
) api.FederationInternalAPI {
	cfg := &base.Cfg.FederationAPI

	federationDB, err := storage.NewDatabase(base, &cfg.Database, base.Caches, base.Cfg.Global.IsLocalServerName)
	if err != nil {
		logrus.WithError(err).Panic("failed to connect to federation sender db")
	}

	if resetBlacklist {
		_ = federationDB.RemoveAllServersFromBlacklist()
	}

	stats := statistics.NewStatistics(federationDB, cfg.FederationMaxRetries+1)

	js, nats := base.NATS.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)

	signingInfo := base.Cfg.Global.SigningIdentities()

	queues := queue.NewOutgoingQueues(
		federationDB, base.ProcessContext,
		cfg.Matrix.DisableFederation,
		cfg.Matrix.ServerName, federation, rsAPI, &stats,
		signingInfo,
	)

	rsConsumer := consumers.NewOutputRoomEventConsumer(
		base.ProcessContext, cfg, js, nats, queues,
		federationDB, rsAPI,
	)
	if err = rsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start room server consumer")
	}
	tsConsumer := consumers.NewOutputSendToDeviceConsumer(
		base.ProcessContext, cfg, js, queues, federationDB,
	)
	if err = tsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start send-to-device consumer")
	}
	receiptConsumer := consumers.NewOutputReceiptConsumer(
		base.ProcessContext, cfg, js, queues, federationDB,
	)
	if err = receiptConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start receipt consumer")
	}
	typingConsumer := consumers.NewOutputTypingConsumer(
		base.ProcessContext, cfg, js, queues, federationDB,
	)
	if err = typingConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start typing consumer")
	}
	keyConsumer := consumers.NewKeyChangeConsumer(
		base.ProcessContext, &base.Cfg.KeyServer, js, queues, federationDB, rsAPI,
	)
	if err = keyConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start key server consumer")
	}

	presenceConsumer := consumers.NewOutputPresenceConsumer(
		base.ProcessContext, cfg, js, queues, federationDB, rsAPI,
	)
	if err = presenceConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start presence consumer")
	}

	var cleanExpiredEDUs func()
	cleanExpiredEDUs = func() {
		logrus.Infof("Cleaning expired EDUs")
		if err := federationDB.DeleteExpiredEDUs(base.Context()); err != nil {
			logrus.WithError(err).Error("Failed to clean expired EDUs")
		}
		time.AfterFunc(time.Hour, cleanExpiredEDUs)
	}
	time.AfterFunc(time.Minute, cleanExpiredEDUs)

	return internal.NewFederationInternalAPI(federationDB, cfg, rsAPI, federation, &stats, caches, queues, keyRing)
}
