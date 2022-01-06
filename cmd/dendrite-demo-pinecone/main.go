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
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/embed"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/rooms"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
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
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"

	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"

	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
)

var (
	instanceName   = flag.String("name", "dendrite-p2p-pinecone", "the name of this P2P demo instance")
	instancePort   = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer   = flag.String("peer", "", "the static Pinecone peers to connect to, comma separated-list")
	instanceListen = flag.String("listen", ":0", "the port Pinecone peers can connect to")
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
	pRouter := pineconeRouter.NewRouter(logger, sk, false)

	go func() {
		listener, err := net.Listen("tcp", *instanceListen)
		if err != nil {
			panic(err)
		}

		fmt.Println("Listening on", listener.Addr())

		for {
			conn, err := listener.Accept()
			if err != nil {
				logrus.WithError(err).Error("listener.Accept failed")
				continue
			}

			port, err := pRouter.Connect(
				conn,
				pineconeRouter.ConnectionPeerType(pineconeRouter.PeerTypeRemote),
			)
			if err != nil {
				logrus.WithError(err).Error("pSwitch.Connect failed")
				continue
			}

			fmt.Println("Inbound connection", conn.RemoteAddr(), "is connected to port", port)
		}
	}()

	pQUIC := pineconeSessions.NewSessions(logger, pRouter)
	pMulticast := pineconeMulticast.NewMulticast(logger, pRouter)
	pMulticast.Start()

	connectToStaticPeer := func() {
		connected := map[string]bool{} // URI -> connected?
		for _, uri := range strings.Split(*instancePeer, ",") {
			connected[strings.TrimSpace(uri)] = false
		}
		attempt := func() {
			for k := range connected {
				connected[k] = false
			}
			for _, info := range pRouter.Peers() {
				connected[info.URI] = true
			}
			for k, online := range connected {
				if !online {
					if err := conn.ConnectToPeer(pRouter, k); err != nil {
						logrus.WithError(err).Error("Failed to connect to static peer")
					}
				}
			}
		}
		for {
			attempt()
			time.Sleep(time.Second * 5)
		}
	}

	cfg := &config.Dendrite{}
	cfg.Defaults(true)
	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.PrivateKey = sk
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.JetStream.StoragePath = config.Path(fmt.Sprintf("%s/", *instanceName))
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", *instanceName))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationapi.db", *instanceName))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.MSCs.MSCs = []string{"msc2836", "msc2946"}
	if err := cfg.Derive(); err != nil {
		panic(err)
	}

	base := base.NewBaseDendrite(cfg, "Monolith")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := conn.CreateFederationClient(base, pQUIC)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	rsComponent := roomserver.NewInternalAPI(base)
	rsAPI := rsComponent
	fsAPI := federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, keyRing, true,
	)

	keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, fsAPI)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, nil, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), userAPI,
	)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)

	rsComponent.SetFederationAPI(fsAPI, keyRing)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    conn.CreateClient(base, pQUIC),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:          asAPI,
		EDUInternalAPI:         eduInputAPI,
		FederationAPI:          fsAPI,
		RoomserverAPI:          rsAPI,
		UserAPI:                userAPI,
		KeyAPI:                 keyAPI,
		ExtPublicRoomsProvider: rooms.NewPineconeRoomProvider(pRouter, pQUIC, fsAPI, federation),
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

	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			return true
		},
	}
	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)
	httpRouter.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			logrus.WithError(err).Error("Failed to upgrade WebSocket connection")
			return
		}
		conn := conn.WrapWebSocketConn(c)
		if _, err = pRouter.Connect(
			conn,
			pineconeRouter.ConnectionZone("websocket"),
			pineconeRouter.ConnectionPeerType(pineconeRouter.PeerTypeRemote),
		); err != nil {
			logrus.WithError(err).Error("Failed to connect WebSocket peer to Pinecone switch")
		}
	})
	embed.Embed(httpRouter, *instancePort, "Pinecone Demo")

	pMux := mux.NewRouter().SkipClean(true).UseEncodedPath()
	pMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	pMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	pHTTP := pQUIC.HTTP()
	pHTTP.Mux().Handle(httputil.PublicFederationPathPrefix, pMux)
	pHTTP.Mux().Handle(httputil.PublicMediaPathPrefix, pMux)

	// Build both ends of a HTTP multiplex.
	httpServer := &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
		Handler: pMux,
	}

	go connectToStaticPeer()
	go func() {
		pubkey := pRouter.PublicKey()
		logrus.Info("Listening on ", hex.EncodeToString(pubkey[:]))
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

	base.WaitForShutdown()
}
