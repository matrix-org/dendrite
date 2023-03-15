// Copyright 2023 The Matrix.org Foundation C.I.C.
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

package monolith

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/embed"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/relay"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/rooms"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/users"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/federationapi"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/relayapi"
	relayAPI "github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi"
	userAPI "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"

	pineconeConnections "github.com/matrix-org/pinecone/connections"
	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeEvents "github.com/matrix-org/pinecone/router/events"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

const SessionProtocol = "matrix"

type P2PMonolith struct {
	BaseDendrite   *base.BaseDendrite
	Sessions       *pineconeSessions.Sessions
	Multicast      *pineconeMulticast.Multicast
	ConnManager    *pineconeConnections.ConnectionManager
	Router         *pineconeRouter.Router
	EventChannel   chan pineconeEvents.Event
	RelayRetriever relay.RelayServerRetriever

	dendrite           setup.Monolith
	port               int
	httpMux            *mux.Router
	pineconeMux        *mux.Router
	httpServer         *http.Server
	listener           net.Listener
	httpListenAddr     string
	stopHandlingEvents chan bool
}

func GenerateDefaultConfig(sk ed25519.PrivateKey, storageDir string, cacheDir string, dbPrefix string) *config.Dendrite {
	cfg := config.Dendrite{}
	cfg.Defaults(config.DefaultOpts{
		Generate:       true,
		SingleDatabase: true,
	})
	cfg.Global.PrivateKey = sk
	cfg.Global.JetStream.StoragePath = config.Path(fmt.Sprintf("%s/", filepath.Join(cacheDir, dbPrefix)))
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", filepath.Join(storageDir, dbPrefix)))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", filepath.Join(storageDir, dbPrefix)))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", filepath.Join(storageDir, dbPrefix)))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", filepath.Join(storageDir, dbPrefix)))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", filepath.Join(storageDir, dbPrefix)))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", filepath.Join(storageDir, dbPrefix)))
	cfg.RelayAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-relayapi.db", filepath.Join(storageDir, dbPrefix)))
	cfg.MSCs.MSCs = []string{"msc2836", "msc2946"}
	cfg.MSCs.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mscs.db", filepath.Join(storageDir, dbPrefix)))
	cfg.ClientAPI.RegistrationDisabled = false
	cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
	cfg.MediaAPI.BasePath = config.Path(filepath.Join(cacheDir, "media"))
	cfg.MediaAPI.AbsBasePath = config.Path(filepath.Join(cacheDir, "media"))
	cfg.SyncAPI.Fulltext.Enabled = true
	cfg.SyncAPI.Fulltext.IndexPath = config.Path(filepath.Join(cacheDir, "search"))
	if err := cfg.Derive(); err != nil {
		panic(err)
	}

	return &cfg
}

func (p *P2PMonolith) SetupPinecone(sk ed25519.PrivateKey) {
	p.EventChannel = make(chan pineconeEvents.Event)
	p.Router = pineconeRouter.NewRouter(logrus.WithField("pinecone", "router"), sk)
	p.Router.EnableHopLimiting()
	p.Router.EnableWakeupBroadcasts()
	p.Router.Subscribe(p.EventChannel)

	p.Sessions = pineconeSessions.NewSessions(logrus.WithField("pinecone", "sessions"), p.Router, []string{SessionProtocol})
	p.Multicast = pineconeMulticast.NewMulticast(logrus.WithField("pinecone", "multicast"), p.Router)
	p.ConnManager = pineconeConnections.NewConnectionManager(p.Router, nil)
}

func (p *P2PMonolith) SetupDendrite(cfg *config.Dendrite, port int, enableRelaying bool, enableMetrics bool, enableWebsockets bool) {
	if enableMetrics {
		p.BaseDendrite = base.NewBaseDendrite(cfg)
	} else {
		p.BaseDendrite = base.NewBaseDendrite(cfg, base.DisableMetrics)
	}
	p.port = port
	p.BaseDendrite.ConfigureAdminEndpoints()

	federation := conn.CreateFederationClient(p.BaseDendrite, p.Sessions)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	caches := caching.NewRistrettoCache(cfg.Global.Cache.EstimatedMaxSize, cfg.Global.Cache.MaxAge, enableMetrics)
	rsAPI := roomserver.NewInternalAPI(p.BaseDendrite, caches)
	fsAPI := federationapi.NewInternalAPI(
		p.BaseDendrite, federation, rsAPI, caches, keyRing, true,
	)

	userAPI := userapi.NewInternalAPI(p.BaseDendrite, rsAPI, federation)

	asAPI := appservice.NewInternalAPI(p.BaseDendrite, userAPI, rsAPI)

	rsAPI.SetFederationAPI(fsAPI, keyRing)

	userProvider := users.NewPineconeUserProvider(p.Router, p.Sessions, userAPI, federation)
	roomProvider := rooms.NewPineconeRoomProvider(p.Router, p.Sessions, fsAPI, federation)

	js, _ := p.BaseDendrite.NATS.Prepare(p.BaseDendrite.ProcessContext, &p.BaseDendrite.Cfg.Global.JetStream)
	producer := &producers.SyncAPIProducer{
		JetStream:              js,
		TopicReceiptEvent:      p.BaseDendrite.Cfg.Global.JetStream.Prefixed(jetstream.OutputReceiptEvent),
		TopicSendToDeviceEvent: p.BaseDendrite.Cfg.Global.JetStream.Prefixed(jetstream.OutputSendToDeviceEvent),
		TopicTypingEvent:       p.BaseDendrite.Cfg.Global.JetStream.Prefixed(jetstream.OutputTypingEvent),
		TopicPresenceEvent:     p.BaseDendrite.Cfg.Global.JetStream.Prefixed(jetstream.OutputPresenceEvent),
		TopicDeviceListUpdate:  p.BaseDendrite.Cfg.Global.JetStream.Prefixed(jetstream.InputDeviceListUpdate),
		TopicSigningKeyUpdate:  p.BaseDendrite.Cfg.Global.JetStream.Prefixed(jetstream.InputSigningKeyUpdate),
		Config:                 &p.BaseDendrite.Cfg.FederationAPI,
		UserAPI:                userAPI,
	}
	relayAPI := relayapi.NewRelayInternalAPI(p.BaseDendrite, federation, rsAPI, keyRing, producer, enableRelaying, caches)
	logrus.Infof("Relaying enabled: %v", relayAPI.RelayingEnabled())

	p.dendrite = setup.Monolith{
		Config:    p.BaseDendrite.Cfg,
		Client:    conn.CreateClient(p.BaseDendrite, p.Sessions),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:            asAPI,
		FederationAPI:            fsAPI,
		RoomserverAPI:            rsAPI,
		UserAPI:                  userAPI,
		RelayAPI:                 relayAPI,
		ExtPublicRoomsProvider:   roomProvider,
		ExtUserDirectoryProvider: userProvider,
	}
	p.dendrite.AddAllPublicRoutes(p.BaseDendrite, caches)

	p.setupHttpServers(userProvider, enableWebsockets)
}

func (p *P2PMonolith) GetFederationAPI() federationAPI.FederationInternalAPI {
	return p.dendrite.FederationAPI
}

func (p *P2PMonolith) GetRelayAPI() relayAPI.RelayInternalAPI {
	return p.dendrite.RelayAPI
}

func (p *P2PMonolith) GetUserAPI() userAPI.UserInternalAPI {
	return p.dendrite.UserAPI
}

func (p *P2PMonolith) StartMonolith() {
	p.startHTTPServers()
	p.startEventHandler()
}

func (p *P2PMonolith) Stop() {
	logrus.Info("Stopping monolith")
	_ = p.BaseDendrite.Close()
	p.WaitForShutdown()
	logrus.Info("Stopped monolith")
}

func (p *P2PMonolith) WaitForShutdown() {
	p.BaseDendrite.WaitForShutdown()
	p.closeAllResources()
}

func (p *P2PMonolith) closeAllResources() {
	logrus.Info("Closing monolith resources")
	if p.httpServer != nil {
		_ = p.httpServer.Shutdown(context.Background())
	}

	select {
	case p.stopHandlingEvents <- true:
	default:
	}

	if p.listener != nil {
		_ = p.listener.Close()
	}

	if p.Multicast != nil {
		p.Multicast.Stop()
	}

	if p.Sessions != nil {
		_ = p.Sessions.Close()
	}

	if p.Router != nil {
		_ = p.Router.Close()
	}
	logrus.Info("Monolith resources closed")
}

func (p *P2PMonolith) Addr() string {
	return p.httpListenAddr
}

func (p *P2PMonolith) setupHttpServers(userProvider *users.PineconeUserProvider, enableWebsockets bool) {
	p.httpMux = mux.NewRouter().SkipClean(true).UseEncodedPath()
	p.httpMux.PathPrefix(httputil.PublicClientPathPrefix).Handler(p.BaseDendrite.PublicClientAPIMux)
	p.httpMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(p.BaseDendrite.PublicMediaAPIMux)
	p.httpMux.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(p.BaseDendrite.DendriteAdminMux)
	p.httpMux.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(p.BaseDendrite.SynapseAdminMux)

	if enableWebsockets {
		wsUpgrader := websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		}
		p.httpMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
			c, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				logrus.WithError(err).Error("Failed to upgrade WebSocket connection")
				return
			}
			conn := conn.WrapWebSocketConn(c)
			if _, err = p.Router.Connect(
				conn,
				pineconeRouter.ConnectionZone("websocket"),
				pineconeRouter.ConnectionPeerType(pineconeRouter.PeerTypeRemote),
			); err != nil {
				logrus.WithError(err).Error("Failed to connect WebSocket peer to Pinecone switch")
			}
		})
	}

	p.httpMux.HandleFunc("/pinecone", p.Router.ManholeHandler)

	if enableWebsockets {
		embed.Embed(p.httpMux, p.port, "Pinecone Demo")
	}

	p.pineconeMux = mux.NewRouter().SkipClean(true).UseEncodedPath()
	p.pineconeMux.PathPrefix(users.PublicURL).HandlerFunc(userProvider.FederatedUserProfiles)
	p.pineconeMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(p.BaseDendrite.PublicFederationAPIMux)
	p.pineconeMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(p.BaseDendrite.PublicMediaAPIMux)

	pHTTP := p.Sessions.Protocol(SessionProtocol).HTTP()
	pHTTP.Mux().Handle(users.PublicURL, p.pineconeMux)
	pHTTP.Mux().Handle(httputil.PublicFederationPathPrefix, p.pineconeMux)
	pHTTP.Mux().Handle(httputil.PublicMediaPathPrefix, p.pineconeMux)
}

func (p *P2PMonolith) startHTTPServers() {
	go func() {
		// Build both ends of a HTTP multiplex.
		p.httpServer = &http.Server{
			Addr:         ":0",
			TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
			BaseContext: func(_ net.Listener) context.Context {
				return context.Background()
			},
			Handler: p.pineconeMux,
		}

		pubkey := p.Router.PublicKey()
		pubkeyString := hex.EncodeToString(pubkey[:])
		logrus.Info("Listening on ", pubkeyString)

		switch p.httpServer.Serve(p.Sessions.Protocol(SessionProtocol)) {
		case net.ErrClosed, http.ErrServerClosed:
			logrus.Info("Stopped listening on ", pubkeyString)
		default:
			logrus.Error("Stopped listening on ", pubkeyString)
		}
		logrus.Info("Stopped goroutine listening on ", pubkeyString)
	}()

	p.httpListenAddr = fmt.Sprintf(":%d", p.port)
	go func() {
		logrus.Info("Listening on ", p.httpListenAddr)
		switch http.ListenAndServe(p.httpListenAddr, p.httpMux) {
		case net.ErrClosed, http.ErrServerClosed:
			logrus.Info("Stopped listening on ", p.httpListenAddr)
		default:
			logrus.Error("Stopped listening on ", p.httpListenAddr)
		}
		logrus.Info("Stopped goroutine listening on ", p.httpListenAddr)
	}()
}

func (p *P2PMonolith) startEventHandler() {
	p.stopHandlingEvents = make(chan bool)
	stopRelayServerSync := make(chan bool)
	eLog := logrus.WithField("pinecone", "events")
	p.RelayRetriever = relay.NewRelayServerRetriever(
		context.Background(),
		gomatrixserverlib.ServerName(p.Router.PublicKey().String()),
		p.dendrite.FederationAPI,
		p.dendrite.RelayAPI,
		stopRelayServerSync,
	)
	p.RelayRetriever.InitializeRelayServers(eLog)

	go func(ch <-chan pineconeEvents.Event) {
		for {
			select {
			case event := <-ch:
				switch e := event.(type) {
				case pineconeEvents.PeerAdded:
					p.RelayRetriever.StartSync()
				case pineconeEvents.PeerRemoved:
					if p.RelayRetriever.IsRunning() && p.Router.TotalPeerCount() == 0 {
						// NOTE: Don't block on channel
						select {
						case stopRelayServerSync <- true:
						default:
						}
					}
				case pineconeEvents.BroadcastReceived:
					// eLog.Info("Broadcast received from: ", e.PeerID)

					req := &federationAPI.PerformWakeupServersRequest{
						ServerNames: []gomatrixserverlib.ServerName{gomatrixserverlib.ServerName(e.PeerID)},
					}
					res := &federationAPI.PerformWakeupServersResponse{}
					if err := p.dendrite.FederationAPI.PerformWakeupServers(p.BaseDendrite.Context(), req, res); err != nil {
						eLog.WithError(err).Error("Failed to wakeup destination", e.PeerID)
					}
				}
			case <-p.stopHandlingEvents:
				logrus.Info("Stopping processing pinecone events")
				// NOTE: Don't block on channel
				select {
				case stopRelayServerSync <- true:
				default:
				}
				logrus.Info("Stopped processing pinecone events")
				return
			}
		}
	}(p.EventChannel)
}
