package gobind

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggrooms"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"

	_ "golang.org/x/mobile/bind"
)

type DendriteMonolith struct {
	logger           logrus.Logger
	YggdrasilNode    *yggconn.Node
	StorageDirectory string
	listener         net.Listener
	httpServer       *http.Server
	processContext   *process.ProcessContext
}

func (m *DendriteMonolith) BaseURL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *DendriteMonolith) PeerCount() int {
	return m.YggdrasilNode.PeerCount()
}

func (m *DendriteMonolith) SetMulticastEnabled(enabled bool) {
	m.YggdrasilNode.SetMulticastEnabled(enabled)
}

func (m *DendriteMonolith) SetStaticPeer(uri string) error {
	return m.YggdrasilNode.SetStaticPeer(uri)
}

func (m *DendriteMonolith) DisconnectNonMulticastPeers() {
	m.YggdrasilNode.DisconnectNonMulticastPeers()
}

func (m *DendriteMonolith) DisconnectMulticastPeers() {
	m.YggdrasilNode.DisconnectMulticastPeers()
}

func (m *DendriteMonolith) Start() {
	m.logger = logrus.Logger{
		Out: BindLogger{},
	}
	m.logger.SetOutput(BindLogger{})
	logrus.SetOutput(BindLogger{})

	var err error
	m.listener, err = net.Listen("tcp", "localhost:65432")
	if err != nil {
		panic(err)
	}

	ygg, err := yggconn.Setup("dendrite", m.StorageDirectory, "")
	if err != nil {
		panic(err)
	}
	m.YggdrasilNode = ygg

	cfg := &config.Dendrite{}
	cfg.Defaults(true)
	cfg.Global.ServerName = gomatrixserverlib.ServerName(ygg.DerivedServerName())
	cfg.Global.PrivateKey = ygg.PrivateKey()
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.JetStream.StoragePath = config.Path(fmt.Sprintf("%s/", m.StorageDirectory))
	cfg.Global.JetStream.InMemory = true
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-account.db", m.StorageDirectory))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-mediaapi.db", m.StorageDirectory))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-syncapi.db", m.StorageDirectory))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-roomserver.db", m.StorageDirectory))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-keyserver.db", m.StorageDirectory))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-federationsender.db", m.StorageDirectory))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-appservice.db", m.StorageDirectory))
	cfg.MediaAPI.BasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	cfg.MediaAPI.AbsBasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := base.NewBaseDendrite(cfg, "Monolith")
	m.processContext = base.ProcessContext
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := ygg.CreateFederationClient(base)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	rsAPI := roomserver.NewInternalAPI(base)

	fsAPI := federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, keyRing, true,
	)

	keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, federation)
	userAPI := userapi.NewInternalAPI(base, accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI, rsAPI, base.PushGatewayHTTPClient())
	keyAPI.SetUserAPI(userAPI)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
	rsAPI.SetAppserviceAPI(asAPI)

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsAPI.SetFederationAPI(fsAPI, keyRing)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    ygg.CreateClient(base),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI: asAPI,
		FederationAPI: fsAPI,
		RoomserverAPI: rsAPI,
		UserAPI:       userAPI,
		KeyAPI:        keyAPI,
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

	httpRouter := mux.NewRouter()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	yggRouter := mux.NewRouter()
	yggRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	yggRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	// Build both ends of a HTTP multiplex.
	m.httpServer = &http.Server{
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
		m.logger.Info("Listening on ", ygg.DerivedServerName())
		m.logger.Fatal(m.httpServer.Serve(ygg))
	}()
	go func() {
		logrus.Info("Listening on ", m.listener.Addr())
		logrus.Fatal(http.Serve(m.listener, httpRouter))
	}()
	go func() {
		logrus.Info("Sending wake-up message to known nodes")
		req := &api.PerformBroadcastEDURequest{}
		res := &api.PerformBroadcastEDUResponse{}
		if err := fsAPI.PerformBroadcastEDU(context.TODO(), req, res); err != nil {
			logrus.WithError(err).Error("Failed to send wake-up message to known nodes")
		}
	}()
}

func (m *DendriteMonolith) Stop() {
	if err := m.httpServer.Close(); err != nil {
		m.logger.Warn("Error stopping HTTP server:", err)
	}
	if m.processContext != nil {
		m.processContext.ShutdownDendrite()
		m.processContext.WaitForComponentsToFinish()
	}
}
