package gobind

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

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
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"github.com/yggdrasil-network/yggdrasil-go/src/crypto"
	"go.uber.org/atomic"
)

type DendriteMonolith struct {
	logger           logrus.Logger
	YggdrasilNode    *yggconn.Node
	StorageDirectory string
	listener         net.Listener
	httpServer       *http.Server
	httpListening    atomic.Bool
	yggListening     atomic.Bool
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

	ygg, err := yggconn.Setup("dendrite", m.StorageDirectory)
	if err != nil {
		panic(err)
	}
	m.YggdrasilNode = ygg

	cfg := &config.Dendrite{}
	cfg.SetDefaults()
	cfg.Matrix.ServerName = gomatrixserverlib.ServerName(ygg.DerivedServerName())
	cfg.Matrix.PrivateKey = ygg.SigningPrivateKey()
	cfg.Matrix.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Kafka.UseNaffka = true
	cfg.Kafka.Topics.OutputRoomEvent = "roomserverOutput"
	cfg.Kafka.Topics.OutputClientData = "clientapiOutput"
	cfg.Kafka.Topics.OutputTypingEvent = "typingServerOutput"
	cfg.Kafka.Topics.OutputSendToDeviceEvent = "sendToDeviceOutput"
	cfg.Database.Account = config.DataSource(fmt.Sprintf("file:%s/dendrite-account.db", m.StorageDirectory))
	cfg.Database.Device = config.DataSource(fmt.Sprintf("file:%s/dendrite-device.db", m.StorageDirectory))
	cfg.Database.MediaAPI = config.DataSource(fmt.Sprintf("file:%s/dendrite-mediaapi.db", m.StorageDirectory))
	cfg.Database.SyncAPI = config.DataSource(fmt.Sprintf("file:%s/dendrite-syncapi.db", m.StorageDirectory))
	cfg.Database.RoomServer = config.DataSource(fmt.Sprintf("file:%s/dendrite-roomserver.db", m.StorageDirectory))
	cfg.Database.ServerKey = config.DataSource(fmt.Sprintf("file:%s/dendrite-serverkey.db", m.StorageDirectory))
	cfg.Database.FederationSender = config.DataSource(fmt.Sprintf("file:%s/dendrite-federationsender.db", m.StorageDirectory))
	cfg.Database.AppService = config.DataSource(fmt.Sprintf("file:%s/dendrite-appservice.db", m.StorageDirectory))
	cfg.Database.CurrentState = config.DataSource(fmt.Sprintf("file:%s/dendrite-currentstate.db", m.StorageDirectory))
	cfg.Database.Naffka = config.DataSource(fmt.Sprintf("file:%s/dendrite-naffka.db", m.StorageDirectory))
	cfg.Media.BasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	cfg.Media.AbsBasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
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
	userAPI := userapi.NewInternalAPI(accountDB, deviceDB, cfg.Matrix.ServerName, cfg.Derived.ApplicationServices)

	rsAPI := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), userAPI,
	)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsAPI.SetFederationSenderAPI(fsAPI)

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

	ygg.NewSession = func(serverName gomatrixserverlib.ServerName) {
		logrus.Infof("Found new session %q", serverName)
		time.Sleep(time.Second * 3)
		req := &api.PerformServersAliveRequest{
			Servers: []gomatrixserverlib.ServerName{serverName},
		}
		res := &api.PerformServersAliveResponse{}
		if err := fsAPI.PerformServersAlive(context.TODO(), req, res); err != nil {
			logrus.WithError(err).Warn("Failed to notify server alive due to new session")
		}
	}

	ygg.NotifyLinkNew(func(_ crypto.BoxPubKey, sigPubKey crypto.SigPubKey, linkType, remote string) {
		serverName := hex.EncodeToString(sigPubKey[:])
		logrus.Infof("Found new peer %q", serverName)
		time.Sleep(time.Second * 3)
		req := &api.PerformServersAliveRequest{
			Servers: []gomatrixserverlib.ServerName{
				gomatrixserverlib.ServerName(serverName),
			},
		}
		res := &api.PerformServersAliveResponse{}
		if err := fsAPI.PerformServersAlive(context.TODO(), req, res); err != nil {
			logrus.WithError(err).Warn("Failed to notify server alive due to new session")
		}
	})

	// Build both ends of a HTTP multiplex.
	m.httpServer = &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
		Handler: base.BaseMux,
	}

	m.Resume()
}

func (m *DendriteMonolith) Resume() {
	logrus.Info("Resuming monolith")
	if listener, err := net.Listen("tcp", "localhost:65432"); err == nil {
		m.listener = listener
	}
	if m.yggListening.CAS(false, true) {
		go func() {
			m.logger.Info("Listening on ", m.YggdrasilNode.DerivedServerName())
			m.logger.Fatal(m.httpServer.Serve(m.YggdrasilNode))
			m.yggListening.Store(false)
		}()
	}
	if m.httpListening.CAS(false, true) {
		go func() {
			m.logger.Info("Listening on ", m.BaseURL())
			m.logger.Fatal(m.httpServer.Serve(m.listener))
			m.httpListening.Store(false)
		}()
	}
}

func (m *DendriteMonolith) Suspend() {
	m.logger.Info("Suspending monolith")
	if err := m.httpServer.Close(); err != nil {
		m.logger.Warn("Error stopping HTTP server:", err)
	}
}
