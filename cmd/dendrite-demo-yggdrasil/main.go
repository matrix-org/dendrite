// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/embed"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggrooms"
	"github.com/matrix-org/dendrite/currentstateserver"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/sirupsen/logrus"
)

var (
	instanceName = flag.String("name", "dendrite-p2p-ygg", "the name of this P2P demo instance")
	instancePort = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer = flag.String("peer", "", "an internet Yggdrasil peer to connect to")
)

// nolint:gocyclo
func main() {
	flag.Parse()
	internal.SetupPprof()

	ygg, err := yggconn.Setup(*instanceName, ".")
	if err != nil {
		panic(err)
	}
	ygg.SetMulticastEnabled(true)
	if instancePeer != nil && *instancePeer != "" {
		if err = ygg.SetStaticPeer(*instancePeer); err != nil {
			logrus.WithError(err).Error("Failed to set static peer")
		}
	}

	cfg := &config.Dendrite{}
	cfg.SetDefaults()
	cfg.Matrix.ServerName = gomatrixserverlib.ServerName(ygg.DerivedServerName())
	cfg.Matrix.PrivateKey = ygg.SigningPrivateKey()
	cfg.Matrix.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Matrix.FederationMaxRetries = 6
	cfg.Kafka.UseNaffka = true
	cfg.Kafka.Topics.OutputRoomEvent = "roomserverOutput"
	cfg.Kafka.Topics.OutputClientData = "clientapiOutput"
	cfg.Kafka.Topics.OutputTypingEvent = "typingServerOutput"
	cfg.Database.Account = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.Database.Device = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.Database.MediaAPI = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.Database.SyncAPI = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.Database.RoomServer = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.Database.ServerKey = config.DataSource(fmt.Sprintf("file:%s-serverkey.db", *instanceName))
	cfg.Database.FederationSender = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", *instanceName))
	cfg.Database.AppService = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.Database.CurrentState = config.DataSource(fmt.Sprintf("file:%s-currentstate.db", *instanceName))
	cfg.Database.Naffka = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
	cfg.Database.E2EKey = config.DataSource(fmt.Sprintf("file:%s-e2ekey.db", *instanceName))
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := setup.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	federation := ygg.CreateFederationClient(base)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	userAPI := userapi.NewInternalAPI(accountDB, deviceDB, cfg.Matrix.ServerName, nil)

	rsComponent := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)
	rsAPI := rsComponent

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), userAPI,
	)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)

	rsComponent.SetFederationSenderAPI(fsAPI)

	embed.Embed(base.BaseMux, *instancePort, "Yggdrasil Demo")

	stateAPI := currentstateserver.NewInternalAPI(base.Cfg, base.KafkaConsumer)

	monolith := setup.Monolith{
		Config:        base.Cfg,
		AccountDB:     accountDB,
		DeviceDB:      deviceDB,
		Client:        ygg.CreateClient(base),
		FedClient:     federation,
		KeyRing:       keyRing,
		KafkaConsumer: base.KafkaConsumer,
		KafkaProducer: base.KafkaProducer,

		AppserviceAPI:       asAPI,
		EDUInternalAPI:      eduInputAPI,
		FederationSenderAPI: fsAPI,
		RoomserverAPI:       rsAPI,
		UserAPI:             userAPI,
		StateAPI:            stateAPI,
		KeyAPI:              keyserver.NewInternalAPI(base.Cfg, federation, userAPI, base.KafkaProducer),
		//ServerKeyAPI:        serverKeyAPI,
		ExtPublicRoomsProvider: yggrooms.NewYggdrasilRoomProvider(
			ygg, fsAPI, federation,
		),
	}
	monolith.AddAllPublicRoutes(base.PublicAPIMux)

	httputil.SetupHTTPAPI(
		base.BaseMux,
		base.PublicAPIMux,
		base.InternalAPIMux,
		cfg,
		base.UseHTTPAPIs,
	)

	// Build both ends of a HTTP multiplex.
	httpServer := &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
		Handler: base.BaseMux,
	}

	go func() {
		logrus.Info("Listening on ", ygg.DerivedServerName())
		logrus.Fatal(httpServer.Serve(ygg))
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, base.BaseMux))
	}()
	go func() {
		logrus.Info("Sending wake-up message to known nodes")
		req := &api.PerformBroadcastEDURequest{}
		res := &api.PerformBroadcastEDUResponse{}
		if err := fsAPI.PerformBroadcastEDU(context.TODO(), req, res); err != nil {
			logrus.WithError(err).Error("Failed to send wake-up message to known nodes")
		}
	}()

	select {}
}
