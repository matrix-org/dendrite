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

package clientapi

import (
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/api"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/transactions"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/jetstream"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// AddPublicRoutes sets up and registers HTTP handlers for the ClientAPI component.
func AddPublicRoutes(
	base *base.BaseDendrite,
	federation *gomatrixserverlib.FederationClient,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	asAPI appserviceAPI.AppServiceInternalAPI,
	transactionsCache *transactions.Cache,
	fsAPI federationAPI.ClientFederationAPI,
	userAPI userapi.ClientUserAPI,
	userDirectoryProvider userapi.QuerySearchProfilesAPI,
	keyAPI keyserverAPI.ClientKeyAPI,
	extRoomsProvider api.ExtraPublicRoomsProvider,
) {
	cfg := &base.Cfg.ClientAPI
	mscCfg := &base.Cfg.MSCs
	js, natsClient := base.NATS.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)

	syncProducer := &producers.SyncAPIProducer{
		JetStream:              js,
		TopicReceiptEvent:      cfg.Matrix.JetStream.Prefixed(jetstream.OutputReceiptEvent),
		TopicSendToDeviceEvent: cfg.Matrix.JetStream.Prefixed(jetstream.OutputSendToDeviceEvent),
		TopicTypingEvent:       cfg.Matrix.JetStream.Prefixed(jetstream.OutputTypingEvent),
		TopicPresenceEvent:     cfg.Matrix.JetStream.Prefixed(jetstream.OutputPresenceEvent),
		UserAPI:                userAPI,
		ServerName:             cfg.Matrix.ServerName,
	}

	routing.Setup(
		base.PublicClientAPIMux,
		base.PublicWellKnownAPIMux,
		base.SynapseAdminMux,
		base.DendriteAdminMux,
		cfg, rsAPI, asAPI,
		userAPI, userDirectoryProvider, federation,
		syncProducer, transactionsCache, fsAPI, keyAPI,
		extRoomsProvider, mscCfg, natsClient,
	)
}
