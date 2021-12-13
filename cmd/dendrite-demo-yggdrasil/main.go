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
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
)

var (
	instanceName = flag.String("name", "dendrite-p2p-ygg", "the name of this P2P demo instance")
	instancePort = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer = flag.String("peer", "", "an internet Yggdrasil peer to connect to")
)

func main() {
	flag.Parse()
	internal.SetupPprof()

	ygg, err := yggconn.Setup(*instanceName, ".", *instancePeer)
	if err != nil {
		panic(err)
	}
	/*
		ygg.SetMulticastEnabled(true)
		if instancePeer != nil && *instancePeer != "" {
			if err = ygg.SetStaticPeer(*instancePeer); err != nil {
				logrus.WithError(err).Error("Failed to set static peer")
			}
		}
	*/

	cfg := &config.Dendrite{}
	cfg.Defaults(true)
	cfg.Global.ServerName = gomatrixserverlib.ServerName(ygg.DerivedServerName())
	cfg.Global.PrivateKey = ygg.PrivateKey()
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.Kafka.UseNaffka = true
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", *instanceName))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationapi.db", *instanceName))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.Global.Kafka.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
	cfg.MSCs.MSCs = []string{"msc2836"}
	cfg.MSCs.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mscs.db", *instanceName))
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := base.NewBaseDendrite(cfg, "Monolith")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := ygg.CreateFederationClient(base)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, federation)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, nil, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	rsComponent := roomserver.NewInternalAPI(
		base,
	)
	rsAPI := rsComponent

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), userAPI,
	)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
	rsAPI.SetAppserviceAPI(asAPI)
	fsAPI := federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, keyRing, true,
	)

	rsComponent.SetFederationAPI(fsAPI, keyRing)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    ygg.CreateClient(base),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:  asAPI,
		EDUInternalAPI: eduInputAPI,
		FederationAPI:  fsAPI,
		RoomserverAPI:  rsAPI,
		UserAPI:        userAPI,
		KeyAPI:         keyAPI,
		ExtPublicRoomsProvider: yggrooms.NewYggdrasilRoomProvider(
			ygg, fsAPI, federation,
		),
	}
	monolith.AddAllPublicRoutes(
		base.ProcessContext,
		base.PublicClientAPIMux,
		base.PublicFederationAPIMux,
		base.PublicKeyAPIMux,
		base.PublicWellKnownAPIMux,
		base.PublicMediaAPIMux,
		base.SynapseAdminMux,
	)
	if err := mscs.Enable(base, &monolith); err != nil {
		logrus.WithError(err).Fatalf("Failed to enable MSCs")
	}

	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)
	embed.Embed(httpRouter, *instancePort, "Yggdrasil Demo")

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

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	base.WaitForShutdown()
}
