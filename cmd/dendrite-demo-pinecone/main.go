// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/embed"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/monolith"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/relay"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/rooms"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/users"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/relayapi"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeEvents "github.com/matrix-org/pinecone/router/events"

	"github.com/sirupsen/logrus"
)

var (
	instanceName            = flag.String("name", "dendrite-p2p-pinecone", "the name of this P2P demo instance")
	instancePort            = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer            = flag.String("peer", "", "the static Pinecone peers to connect to, comma separated-list")
	instanceListen          = flag.String("listen", ":0", "the port Pinecone peers can connect to")
	instanceDir             = flag.String("dir", ".", "the directory to store the databases in (if --config not specified)")
	instanceRelayingEnabled = flag.Bool("relay", false, "whether to enable store & forward relaying for other nodes")
)

func main() {
	flag.Parse()
	internal.SetupPprof()

	var pk ed25519.PublicKey
	var sk ed25519.PrivateKey

	// iterate through the cli args and check if the config flag was set
	configFlagSet := false
	for _, arg := range os.Args {
		if arg == "--config" || arg == "-config" {
			configFlagSet = true
			break
		}
	}

	var cfg *config.Dendrite

	// use custom config if config flag is set
	if configFlagSet {
		cfg = setup.ParseFlags(true)
		sk = cfg.Global.PrivateKey
		pk = sk.Public().(ed25519.PublicKey)
	} else {
		keyfile := filepath.Join(*instanceDir, *instanceName) + ".pem"
		oldKeyfile := *instanceName + ".key"
		sk, pk = monolith.GetOrCreateKey(keyfile, oldKeyfile)
		cfg = monolith.GenerateDefaultConfig(sk, *instanceDir, *instanceName)
	}

	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)

	base := base.NewBaseDendrite(cfg, "Monolith")
	base.ConfigureAdminEndpoints()
	defer base.Close() // nolint: errcheck

	p2pMonolith := monolith.P2PMonolith{}
	p2pMonolith.SetupPinecone(sk)
	p2pMonolith.Multicast.Start()

	if instancePeer != nil && *instancePeer != "" {
		for _, peer := range strings.Split(*instancePeer, ",") {
			p2pMonolith.ConnManager.AddPeer(strings.Trim(peer, " \t\r\n"))
		}
	}

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

			port, err := p2pMonolith.Router.Connect(
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

	// TODO : factor this dendrite setup out to a common place
	federation := conn.CreateFederationClient(base, p2pMonolith.Sessions)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	rsComponent := roomserver.NewInternalAPI(base)
	rsAPI := rsComponent
	fsAPI := federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, keyRing, true,
	)

	keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, fsAPI, rsComponent)
	userAPI := userapi.NewInternalAPI(base, &cfg.UserAPI, nil, keyAPI, rsAPI, base.PushGatewayHTTPClient())
	keyAPI.SetUserAPI(userAPI)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)

	rsComponent.SetFederationAPI(fsAPI, keyRing)

	userProvider := users.NewPineconeUserProvider(p2pMonolith.Router, p2pMonolith.Sessions, userAPI, federation)
	roomProvider := rooms.NewPineconeRoomProvider(p2pMonolith.Router, p2pMonolith.Sessions, fsAPI, federation)

	js, _ := base.NATS.Prepare(base.ProcessContext, &base.Cfg.Global.JetStream)
	producer := &producers.SyncAPIProducer{
		JetStream:              js,
		TopicReceiptEvent:      base.Cfg.Global.JetStream.Prefixed(jetstream.OutputReceiptEvent),
		TopicSendToDeviceEvent: base.Cfg.Global.JetStream.Prefixed(jetstream.OutputSendToDeviceEvent),
		TopicTypingEvent:       base.Cfg.Global.JetStream.Prefixed(jetstream.OutputTypingEvent),
		TopicPresenceEvent:     base.Cfg.Global.JetStream.Prefixed(jetstream.OutputPresenceEvent),
		TopicDeviceListUpdate:  base.Cfg.Global.JetStream.Prefixed(jetstream.InputDeviceListUpdate),
		TopicSigningKeyUpdate:  base.Cfg.Global.JetStream.Prefixed(jetstream.InputSigningKeyUpdate),
		Config:                 &base.Cfg.FederationAPI,
		UserAPI:                userAPI,
	}
	relayAPI := relayapi.NewRelayInternalAPI(base, federation, rsAPI, keyRing, producer, *instanceRelayingEnabled)
	logrus.Infof("Relaying enabled: %v", relayAPI.RelayingEnabled())

	m := setup.Monolith{
		Config:    base.Cfg,
		Client:    conn.CreateClient(base, p2pMonolith.Sessions),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:            asAPI,
		FederationAPI:            fsAPI,
		RoomserverAPI:            rsAPI,
		UserAPI:                  userAPI,
		KeyAPI:                   keyAPI,
		RelayAPI:                 relayAPI,
		ExtPublicRoomsProvider:   roomProvider,
		ExtUserDirectoryProvider: userProvider,
	}
	m.AddAllPublicRoutes(base)

	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			return true
		},
	}
	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)
	httpRouter.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(base.DendriteAdminMux)
	httpRouter.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(base.SynapseAdminMux)
	httpRouter.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			logrus.WithError(err).Error("Failed to upgrade WebSocket connection")
			return
		}
		conn := conn.WrapWebSocketConn(c)
		if _, err = p2pMonolith.Router.Connect(
			conn,
			pineconeRouter.ConnectionZone("websocket"),
			pineconeRouter.ConnectionPeerType(pineconeRouter.PeerTypeRemote),
		); err != nil {
			logrus.WithError(err).Error("Failed to connect WebSocket peer to Pinecone switch")
		}
	})
	httpRouter.HandleFunc("/pinecone", p2pMonolith.Router.ManholeHandler)
	embed.Embed(httpRouter, *instancePort, "Pinecone Demo")

	pMux := mux.NewRouter().SkipClean(true).UseEncodedPath()
	pMux.PathPrefix(users.PublicURL).HandlerFunc(userProvider.FederatedUserProfiles)
	pMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	pMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	pHTTP := p2pMonolith.Sessions.Protocol("matrix").HTTP()
	pHTTP.Mux().Handle(users.PublicURL, pMux)
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

	// TODO : factor these funcs out to a common place
	go func() {
		pubkey := p2pMonolith.Router.PublicKey()
		logrus.Info("Listening on ", hex.EncodeToString(pubkey[:]))
		logrus.Fatal(httpServer.Serve(p2pMonolith.Sessions.Protocol("matrix")))
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, httpRouter))
	}()

	stopRelayServerSync := make(chan bool)
	eLog := logrus.WithField("pinecone", "events")
	relayRetriever := relay.NewRelayServerRetriever(
		context.Background(),
		gomatrixserverlib.ServerName(p2pMonolith.Router.PublicKey().String()),
		m.FederationAPI,
		m.RelayAPI,
		stopRelayServerSync,
	)
	relayRetriever.InitializeRelayServers(eLog)

	go func(ch <-chan pineconeEvents.Event) {
		for event := range ch {
			switch e := event.(type) {
			case pineconeEvents.PeerAdded:
				relayRetriever.StartSync()
			case pineconeEvents.PeerRemoved:
				if relayRetriever.IsRunning() && p2pMonolith.Router.TotalPeerCount() == 0 {
					stopRelayServerSync <- true
				}
			case pineconeEvents.BroadcastReceived:
				// eLog.Info("Broadcast received from: ", e.PeerID)

				req := &api.PerformWakeupServersRequest{
					ServerNames: []gomatrixserverlib.ServerName{gomatrixserverlib.ServerName(e.PeerID)},
				}
				res := &api.PerformWakeupServersResponse{}
				if err := m.FederationAPI.PerformWakeupServers(base.Context(), req, res); err != nil {
					eLog.WithError(err).Error("Failed to wakeup destination", e.PeerID)
				}
			default:
			}
		}
	}(p2pMonolith.EventChannel)

	base.WaitForShutdown()
}
