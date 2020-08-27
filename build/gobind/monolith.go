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
	"github.com/matrix-org/dendrite/currentstateserver"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type DendriteMonolith struct {
	logger           logrus.Logger
	YggdrasilNode    *yggconn.Node
	StorageDirectory string
	listener         net.Listener
	httpServer       *http.Server
}

func (m *DendriteMonolith) BaseURL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *DendriteMonolith) PeerCount() int {
	return m.YggdrasilNode.PeerCount()
}

func (m *DendriteMonolith) SessionCount() int {
	return m.YggdrasilNode.SessionCount()
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

	ygg, err := yggconn.Setup("dendrite", m.StorageDirectory)
	if err != nil {
		panic(err)
	}
	m.YggdrasilNode = ygg

	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.ServerName = gomatrixserverlib.ServerName(ygg.DerivedServerName())
	cfg.Global.PrivateKey = ygg.SigningPrivateKey()
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.Kafka.UseNaffka = true
	cfg.Global.Kafka.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-naffka.db", m.StorageDirectory))
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-account.db", m.StorageDirectory))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-device.db", m.StorageDirectory))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-mediaapi.db", m.StorageDirectory))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-syncapi.db", m.StorageDirectory))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-roomserver.db", m.StorageDirectory))
	cfg.ServerKeyAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-serverkey.db", m.StorageDirectory))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-keyserver.db", m.StorageDirectory))
	cfg.FederationSender.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-federationsender.db", m.StorageDirectory))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-appservice.db", m.StorageDirectory))
	cfg.CurrentStateServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-currentstate.db", m.StorageDirectory))
	cfg.MediaAPI.BasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	cfg.MediaAPI.AbsBasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	cfg.FederationSender.FederationMaxRetries = 8
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := setup.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := ygg.CreateFederationClient(base)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, federation, base.KafkaProducer)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	rsAPI := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)

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

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsAPI.SetFederationSenderAPI(fsAPI)

	monolith := setup.Monolith{
		Config:        base.Cfg,
		AccountDB:     accountDB,
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

func (m *DendriteMonolith) Suspend() {
	m.logger.Info("Suspending monolith")
	if err := m.httpServer.Close(); err != nil {
		m.logger.Warn("Error stopping HTTP server:", err)
	}
}
