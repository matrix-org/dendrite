package gobind

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggrooms"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	basepkg "github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
	var pk ed25519.PublicKey
	var sk ed25519.PrivateKey

	m.logger = logrus.Logger{
		Out: BindLogger{},
	}
	m.logger.SetOutput(BindLogger{})
	logrus.SetOutput(BindLogger{})

	keyfile := filepath.Join(m.StorageDirectory, "p2p.pem")
	if _, err := os.Stat(keyfile); os.IsNotExist(err) {
		oldkeyfile := filepath.Join(m.StorageDirectory, "p2p.key")
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

	var err error
	m.listener, err = net.Listen("tcp", "localhost:65432")
	if err != nil {
		panic(err)
	}

	ygg, err := yggconn.Setup(sk, "dendrite", m.StorageDirectory, "", "")
	if err != nil {
		panic(err)
	}
	m.YggdrasilNode = ygg

	cfg := &config.Dendrite{}
	cfg.Defaults(config.DefaultOpts{
		Generate:       true,
		SingleDatabase: true,
	})
	cfg.Global.ServerName = spec.ServerName(hex.EncodeToString(pk))
	cfg.Global.PrivateKey = sk
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.JetStream.StoragePath = config.Path(fmt.Sprintf("%s/", m.StorageDirectory))
	cfg.Global.JetStream.InMemory = true
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-account.db", m.StorageDirectory))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-mediaapi.db", m.StorageDirectory))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-syncapi.db", m.StorageDirectory))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-roomserver.db", m.StorageDirectory))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-keyserver.db", m.StorageDirectory))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-federationsender.db", m.StorageDirectory))
	cfg.MediaAPI.BasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	cfg.MediaAPI.AbsBasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	cfg.ClientAPI.RegistrationDisabled = false
	cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	configErrors := &config.ConfigErrors{}
	cfg.Verify(configErrors)
	if len(*configErrors) > 0 {
		for _, err := range *configErrors {
			logrus.Errorf("Configuration error: %s", err)
		}
		logrus.Fatalf("Failed to start due to configuration errors")
	}

	internal.SetupStdLogging()
	internal.SetupHookLogging(cfg.Logging)
	internal.SetupPprof()

	logrus.Infof("Dendrite version %s", internal.VersionString())

	if !cfg.ClientAPI.RegistrationDisabled && cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled {
		logrus.Warn("Open registration is enabled")
	}

	closer, err := cfg.SetupTracing()
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}
	defer closer.Close()

	if cfg.Global.Sentry.Enabled {
		logrus.Info("Setting up Sentry for debugging...")
		err = sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.Global.Sentry.DSN,
			Environment:      cfg.Global.Sentry.Environment,
			Debug:            true,
			ServerName:       string(cfg.Global.ServerName),
			Release:          "dendrite@" + internal.VersionString(),
			AttachStacktrace: true,
		})
		if err != nil {
			logrus.WithError(err).Panic("failed to start Sentry")
		}
	}
	processCtx := process.NewProcessContext()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	routers := httputil.NewRouters()
	basepkg.ConfigureAdminEndpoints(processCtx, routers)
	m.processContext = processCtx
	defer func() {
		processCtx.ShutdownDendrite()
		processCtx.WaitForShutdown()
	}() // nolint: errcheck

	federation := ygg.CreateFederationClient(cfg)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	caches := caching.NewRistrettoCache(cfg.Global.Cache.EstimatedMaxSize, cfg.Global.Cache.MaxAge, caching.EnableMetrics)
	natsInstance := jetstream.NATSInstance{}
	rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.EnableMetrics)

	fsAPI := federationapi.NewInternalAPI(
		processCtx, cfg, cm, &natsInstance, federation, rsAPI, caches, keyRing, true,
	)

	userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, federation)

	asAPI := appservice.NewInternalAPI(processCtx, cfg, &natsInstance, userAPI, rsAPI)
	rsAPI.SetAppserviceAPI(asAPI)

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsAPI.SetFederationAPI(fsAPI, keyRing)

	monolith := setup.Monolith{
		Config:    cfg,
		Client:    ygg.CreateClient(),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI: asAPI,
		FederationAPI: fsAPI,
		RoomserverAPI: rsAPI,
		UserAPI:       userAPI,
		ExtPublicRoomsProvider: yggrooms.NewYggdrasilRoomProvider(
			ygg, fsAPI, federation,
		),
	}
	monolith.AddAllPublicRoutes(processCtx, cfg, routers, cm, &natsInstance, caches, caching.EnableMetrics)

	httpRouter := mux.NewRouter()
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(routers.Client)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(routers.Media)
	httpRouter.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(routers.DendriteAdmin)
	httpRouter.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(routers.SynapseAdmin)

	yggRouter := mux.NewRouter()
	yggRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(routers.Federation)
	yggRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(routers.Media)

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
		m.logger.Error(m.httpServer.Serve(ygg))
	}()
	go func() {
		logrus.Info("Listening on ", m.listener.Addr())
		logrus.Error(http.Serve(m.listener, httpRouter))
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
