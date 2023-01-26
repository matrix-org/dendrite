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
	relayServerAPI "github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
	"go.uber.org/atomic"

	pineconeConnections "github.com/matrix-org/pinecone/connections"
	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeEvents "github.com/matrix-org/pinecone/router/events"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"

	"github.com/sirupsen/logrus"
)

var (
	instanceName   = flag.String("name", "dendrite-p2p-pinecone", "the name of this P2P demo instance")
	instancePort   = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer   = flag.String("peer", "", "the static Pinecone peers to connect to, comma separated-list")
	instanceListen = flag.String("listen", ":0", "the port Pinecone peers can connect to")
	instanceDir    = flag.String("dir", ".", "the directory to store the databases in (if --config not specified)")
)

const relayServerRetryInterval = time.Second * 30

// nolint:gocyclo
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

	cfg := &config.Dendrite{}

	// use custom config if config flag is set
	if configFlagSet {
		cfg = setup.ParseFlags(true)
		sk = cfg.Global.PrivateKey
		pk = sk.Public().(ed25519.PublicKey)
	} else {
		keyfile := filepath.Join(*instanceDir, *instanceName) + ".pem"
		if _, err := os.Stat(keyfile); os.IsNotExist(err) {
			oldkeyfile := *instanceName + ".key"
			if _, err = os.Stat(oldkeyfile); os.IsNotExist(err) {
				if err = test.NewMatrixKey(keyfile); err != nil {
					panic("failed to generate a new PEM key: " + err.Error())
				}
				if _, sk, err = config.LoadMatrixKey(keyfile, os.ReadFile); err != nil {
					panic("failed to load PEM key: " + err.Error())
				}
				if len(sk) != ed25519.PrivateKeySize {
					panic("the private key is not long enough")
				}
			} else {
				if sk, err = os.ReadFile(oldkeyfile); err != nil {
					panic("failed to read the old private key: " + err.Error())
				}
				if len(sk) != ed25519.PrivateKeySize {
					panic("the private key is not long enough")
				}
				if err := test.SaveMatrixKey(keyfile, sk); err != nil {
					panic("failed to convert the private key to PEM format: " + err.Error())
				}
			}
		} else {
			var err error
			if _, sk, err = config.LoadMatrixKey(keyfile, os.ReadFile); err != nil {
				panic("failed to load PEM key: " + err.Error())
			}
			if len(sk) != ed25519.PrivateKeySize {
				panic("the private key is not long enough")
			}
		}

		pk = sk.Public().(ed25519.PublicKey)

		cfg.Defaults(config.DefaultOpts{
			Generate:   true,
			Monolithic: true,
		})
		cfg.Global.PrivateKey = sk
		cfg.Global.JetStream.StoragePath = config.Path(fmt.Sprintf("%s/", filepath.Join(*instanceDir, *instanceName)))
		cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.RelayAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-relayapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.MSCs.MSCs = []string{"msc2836", "msc2946"}
		cfg.MSCs.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mscs.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.ClientAPI.RegistrationDisabled = false
		cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
		cfg.MediaAPI.BasePath = config.Path(*instanceDir)
		cfg.SyncAPI.Fulltext.Enabled = true
		cfg.SyncAPI.Fulltext.IndexPath = config.Path(*instanceDir)
		if err := cfg.Derive(); err != nil {
			panic(err)
		}
	}

	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)

	base := base.NewBaseDendrite(cfg, "Monolith")
	base.ConfigureAdminEndpoints()
	defer base.Close() // nolint: errcheck

	pineconeEventChannel := make(chan pineconeEvents.Event)
	pRouter := pineconeRouter.NewRouter(logrus.WithField("pinecone", "router"), sk)
	pRouter.EnableHopLimiting()
	pRouter.EnableWakeupBroadcasts()
	pRouter.Subscribe(pineconeEventChannel)

	pQUIC := pineconeSessions.NewSessions(logrus.WithField("pinecone", "sessions"), pRouter, []string{"matrix"})
	pMulticast := pineconeMulticast.NewMulticast(logrus.WithField("pinecone", "multicast"), pRouter)
	pManager := pineconeConnections.NewConnectionManager(pRouter, nil)
	pMulticast.Start()
	if instancePeer != nil && *instancePeer != "" {
		for _, peer := range strings.Split(*instancePeer, ",") {
			pManager.AddPeer(strings.Trim(peer, " \t\r\n"))
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

	federation := conn.CreateFederationClient(base, pQUIC)

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

	userProvider := users.NewPineconeUserProvider(pRouter, pQUIC, userAPI, federation)
	roomProvider := rooms.NewPineconeRoomProvider(pRouter, pQUIC, fsAPI, federation)

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
	relayAPI := relayapi.NewRelayInternalAPI(base, federation, rsAPI, keyRing, producer)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		Client:    conn.CreateClient(base, pQUIC),
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
	monolith.AddAllPublicRoutes(base)

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
		if _, err = pRouter.Connect(
			conn,
			pineconeRouter.ConnectionZone("websocket"),
			pineconeRouter.ConnectionPeerType(pineconeRouter.PeerTypeRemote),
		); err != nil {
			logrus.WithError(err).Error("Failed to connect WebSocket peer to Pinecone switch")
		}
	})
	httpRouter.HandleFunc("/pinecone", pRouter.ManholeHandler)
	embed.Embed(httpRouter, *instancePort, "Pinecone Demo")

	pMux := mux.NewRouter().SkipClean(true).UseEncodedPath()
	pMux.PathPrefix(users.PublicURL).HandlerFunc(userProvider.FederatedUserProfiles)
	pMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	pMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	pHTTP := pQUIC.Protocol("matrix").HTTP()
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

	go func() {
		pubkey := pRouter.PublicKey()
		logrus.Info("Listening on ", hex.EncodeToString(pubkey[:]))
		logrus.Fatal(httpServer.Serve(pQUIC.Protocol("matrix")))
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, httpRouter))
	}()

	go func(ch <-chan pineconeEvents.Event) {
		eLog := logrus.WithField("pinecone", "events")
		relayServerSyncRunning := atomic.NewBool(false)
		stopRelayServerSync := make(chan bool)

		m := RelayServerRetriever{
			Context:             context.Background(),
			ServerName:          gomatrixserverlib.ServerName(pRouter.PublicKey().String()),
			FederationAPI:       fsAPI,
			RelayServersQueried: make(map[gomatrixserverlib.ServerName]bool),
			RelayAPI:            monolith.RelayAPI,
		}
		m.InitializeRelayServers(eLog)

		for event := range ch {
			switch e := event.(type) {
			case pineconeEvents.PeerAdded:
				if !relayServerSyncRunning.Load() {
					go m.syncRelayServers(stopRelayServerSync, *relayServerSyncRunning)
				}
			case pineconeEvents.PeerRemoved:
				if relayServerSyncRunning.Load() && pRouter.TotalPeerCount() == 0 {
					stopRelayServerSync <- true
				}
			case pineconeEvents.BroadcastReceived:
				// eLog.Info("Broadcast received from: ", e.PeerID)

				req := &api.PerformWakeupServersRequest{
					ServerNames: []gomatrixserverlib.ServerName{gomatrixserverlib.ServerName(e.PeerID)},
				}
				res := &api.PerformWakeupServersResponse{}
				if err := fsAPI.PerformWakeupServers(base.Context(), req, res); err != nil {
					eLog.WithError(err).Error("Failed to wakeup destination", e.PeerID)
				}
			default:
			}
		}
	}(pineconeEventChannel)

	base.WaitForShutdown()
}

type RelayServerRetriever struct {
	Context             context.Context
	ServerName          gomatrixserverlib.ServerName
	FederationAPI       api.FederationInternalAPI
	RelayServersQueried map[gomatrixserverlib.ServerName]bool
	RelayAPI            relayServerAPI.RelayInternalAPI
}

func (m *RelayServerRetriever) InitializeRelayServers(eLog *logrus.Entry) {
	request := api.P2PQueryRelayServersRequest{Server: gomatrixserverlib.ServerName(m.ServerName)}
	response := api.P2PQueryRelayServersResponse{}
	err := m.FederationAPI.P2PQueryRelayServers(m.Context, &request, &response)
	if err != nil {
		eLog.Warnf("Failed obtaining list of this node's relay servers: %s", err.Error())
	}
	for _, server := range response.RelayServers {
		m.RelayServersQueried[server] = false
	}

	eLog.Infof("Registered relay servers: %v", response.RelayServers)
}

func (m *RelayServerRetriever) syncRelayServers(stop <-chan bool, running atomic.Bool) {
	defer running.Store(false)

	t := time.NewTimer(relayServerRetryInterval)
	for {
		relayServersToQuery := []gomatrixserverlib.ServerName{}
		for server, complete := range m.RelayServersQueried {
			if !complete {
				relayServersToQuery = append(relayServersToQuery, server)
			}
		}
		if len(relayServersToQuery) == 0 {
			// All relay servers have been synced.
			return
		}
		m.queryRelayServers(relayServersToQuery)
		t.Reset(relayServerRetryInterval)

		select {
		case <-stop:
			// We have been asked to stop syncing, drain the timer and return.
			if !t.Stop() {
				<-t.C
			}
			return
		case <-t.C:
			// The timer has expired. Continue to the next loop iteration.
		}
	}
}

func (m *RelayServerRetriever) queryRelayServers(relayServers []gomatrixserverlib.ServerName) {
	logrus.Info("querying relay servers for any available transactions")
	for _, server := range relayServers {
		userID, err := gomatrixserverlib.NewUserID("@user:"+string(m.ServerName), false)
		if err != nil {
			return
		}
		err = m.RelayAPI.PerformRelayServerSync(context.Background(), *userID, server)
		if err == nil {
			m.RelayServersQueried[server] = true
			// TODO : What happens if your relay receives new messages after this point?
			// Should you continue to check with them, or should they try and contact you?
			// They could send a "new_async_events" message your way maybe?
			// Then you could mark them as needing to be queried again.
			// What if you miss this message?
			// Maybe you should try querying them again after a certain period of time as a backup?
		} else {
			logrus.Errorf("Failed querying relay server: %s", err.Error())
		}
	}
}
