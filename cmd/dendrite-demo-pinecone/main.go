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
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/embed"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
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

	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeSwitch "github.com/matrix-org/pinecone/packetswitch"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"

	"github.com/sirupsen/logrus"
)

var (
	instanceName = flag.String("name", "dendrite-p2p-pinecone", "the name of this P2P demo instance")
	instancePort = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer = flag.String("peer", "", "the static Pinecone peer to connect to")
)

// nolint:gocyclo
func main() {
	flag.Parse()
	internal.SetupPprof()

	var pk ed25519.PublicKey
	var sk ed25519.PrivateKey

	keyfile := *instanceName + ".key"
	if _, err := os.Stat(keyfile); os.IsNotExist(err) {
		if pk, sk, err = ed25519.GenerateKey(nil); err != nil {
			panic(err)
		}
		if err = ioutil.WriteFile(keyfile, sk, 0644); err != nil {
			panic(err)
		}
	} else if err == nil {
		if sk, err = ioutil.ReadFile(keyfile); err != nil {
			panic(err)
		}
		if len(sk) != ed25519.PrivateKeySize {
			panic("the private key is not long enough")
		}
		pk = sk.Public().(ed25519.PublicKey)
	}

	logger := log.New(os.Stdout, "", 0)

	rL, rR := net.Pipe()
	pSwitch := pineconeSwitch.NewSwitch(logger, sk, pk, false)
	pRouter := pineconeRouter.NewRouter(logger, sk, pk, rL, "router", nil)
	if _, err := pSwitch.Connect(rR); err != nil {
		panic(err)
	}

	if instancePeer != nil && *instancePeer != "" {
		go func(peer string) {
			parent, err := net.Dial("tcp", peer)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to connect to Pinecone static peer")
				return
			}

			if _, err := pSwitch.Connect(parent); err != nil {
				logrus.WithError(err).Errorf("Failed to connect Pinecone static peer to switch")
			}
		}(*instancePeer)
	}

	pQUIC := pineconeSessions.NewQUIC(logger, pRouter)
	_ = pineconeMulticast.NewMulticast(logger, pSwitch)

	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.PrivateKey = sk
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.Kafka.UseNaffka = true
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.SigningKeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-signingkeyserver.db", *instanceName))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", *instanceName))
	cfg.FederationSender.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", *instanceName))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.Global.Kafka.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
	if err := cfg.Derive(); err != nil {
		panic(err)
	}

	base := setup.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := conn.CreateFederationClient(base, pQUIC)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, federation)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, nil, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	rsComponent := roomserver.NewInternalAPI(
		base, keyRing,
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

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    conn.CreateClient(base, pQUIC),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:       asAPI,
		EDUInternalAPI:      eduInputAPI,
		FederationSenderAPI: fsAPI,
		RoomserverAPI:       rsAPI,
		UserAPI:             userAPI,
		KeyAPI:              keyAPI,
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
	embed.Embed(httpRouter, *instancePort, "Pinecone Demo")

	pMux := mux.NewRouter().SkipClean(true).UseEncodedPath()
	pMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	pMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	pQUIC.Mux().Handle(httputil.PublicFederationPathPrefix, pMux)
	pQUIC.Mux().Handle(httputil.PublicMediaPathPrefix, pMux)

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
		Handler: pMux,
	}

	go func() {
		logrus.Info("Listening on ", hex.EncodeToString(pRouter.PublicKey()))
		logrus.Fatal(httpServer.Serve(pQUIC))
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
