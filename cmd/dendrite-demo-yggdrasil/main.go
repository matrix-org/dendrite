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

	"github.com/gorilla/mux"
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
	cfg.Defaults()
	cfg.Global.ServerName = gomatrixserverlib.ServerName(ygg.DerivedServerName())
	cfg.Global.PrivateKey = ygg.SigningPrivateKey()
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.Kafka.UseNaffka = true
	cfg.FederationSender.FederationMaxRetries = 8
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.ServerKeyAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-serverkey.db", *instanceName))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", *instanceName))
	cfg.FederationSender.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", *instanceName))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.CurrentStateServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-currentstate.db", *instanceName))
	cfg.Global.Kafka.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
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

	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, federation, base.KafkaProducer)
	userAPI := userapi.NewInternalAPI(accountDB, deviceDB, cfg.Global.ServerName, nil, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	rsComponent := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)
	rsAPI := rsComponent

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), userAPI,
	)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
	stateAPI := currentstateserver.NewInternalAPI(&base.Cfg.CurrentStateServer, base.KafkaConsumer)
	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, stateAPI, keyRing,
	)

	ygg.SetSessionFunc(func(address string) {
		req := &api.PerformServersAliveRequest{
			Servers: []gomatrixserverlib.ServerName{
				gomatrixserverlib.ServerName(address),
			},
		}
		res := &api.PerformServersAliveResponse{}
		if err := fsAPI.PerformServersAlive(context.TODO(), req, res); err != nil {
			logrus.WithError(err).Error("Failed to send wake-up message to newly connected node")
		}
	})

	rsComponent.SetFederationSenderAPI(fsAPI)

	embed.Embed(base.PublicClientAPIMux, *instancePort, "Yggdrasil Demo")

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
		KeyAPI:              keyAPI,
		ExtPublicRoomsProvider: yggrooms.NewYggdrasilRoomProvider(
			ygg, fsAPI, federation,
		),
	}
	monolith.AddAllPublicRoutes(
		base.PublicClientAPIMux,
		base.PublicFederationAPIMux,
		base.PublicKeyAPIMux,
		base.PublicMediaAPIMux,
	)

	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	yggRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	yggRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	yggRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	// Build both ends of a HTTP multiplex.
	httpServer := &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
		Handler: yggRouter,
	}

	go func() {
		logrus.Info("Listening on ", ygg.DerivedServerName())
		logrus.Fatal(httpServer.Serve(ygg))
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, httpRouter))
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
