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
	"github.com/gorilla/mux"
	eduserverAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationapi/api"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/consumers"
	"github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/inthttp"
	"github.com/matrix-org/dendrite/federationapi/queue"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/internal/caching"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/gomatrixserverlib"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.FederationInternalAPI) {
	inthttp.AddRoutes(intAPI, router)
}

// AddPublicRoutes sets up and registers HTTP handlers on the base API muxes for the FederationAPI component.
func AddPublicRoutes(
	fedRouter, keyRouter, wellKnownRouter *mux.Router,
	cfg *config.FederationAPI,
	userAPI userapi.UserInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	federationAPI federationAPI.FederationInternalAPI,
	eduAPI eduserverAPI.EDUServerInputAPI,
	keyAPI keyserverAPI.KeyInternalAPI,
	mscCfg *config.MSCs,
	servers federationAPI.ServersInRoomProvider,
) {
	routing.Setup(
		fedRouter, keyRouter, wellKnownRouter, cfg, rsAPI,
		eduAPI, federationAPI, keyRing,
		federation, userAPI, keyAPI, mscCfg,
		servers,
	)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite,
	federation *gomatrixserverlib.FederationClient,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	caches *caching.Caches,
	keyRing *gomatrixserverlib.KeyRing,
	resetBlacklist bool,
) api.FederationInternalAPI {
	cfg := &base.Cfg.FederationAPI

	federationDB, err := storage.NewDatabase(&cfg.Database, base.Caches, base.Cfg.Global.ServerName)
	if err != nil {
		logrus.WithError(err).Panic("failed to connect to federation sender db")
	}

	if resetBlacklist {
		_ = federationDB.RemoveAllServersFromBlacklist()
	}

	stats := &statistics.Statistics{
		DB:                     federationDB,
		FailuresUntilBlacklist: cfg.FederationMaxRetries,
	}

	js, _ := jetstream.Prepare(&cfg.Matrix.JetStream)

	queues := queue.NewOutgoingQueues(
		federationDB, base.ProcessContext,
		cfg.Matrix.DisableFederation,
		cfg.Matrix.ServerName, federation, rsAPI, stats,
		&queue.SigningInfo{
			KeyID:      cfg.Matrix.KeyID,
			PrivateKey: cfg.Matrix.PrivateKey,
			ServerName: cfg.Matrix.ServerName,
		},
	)

	rsConsumer := consumers.NewOutputRoomEventConsumer(
		base.ProcessContext, cfg, js, queues,
		federationDB, rsAPI,
	)
	if err = rsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start room server consumer")
	}

	tsConsumer := consumers.NewOutputEDUConsumer(
		base.ProcessContext, cfg, js, queues, federationDB,
	)
	if err := tsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start typing server consumer")
	}
	keyConsumer := consumers.NewKeyChangeConsumer(
		base.ProcessContext, &base.Cfg.KeyServer, js, queues, federationDB, rsAPI,
	)
	if err := keyConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start key server consumer")
	}

	return internal.NewFederationInternalAPI(federationDB, cfg, rsAPI, federation, stats, caches, queues, keyRing)
}
