package gobind

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type DendriteMonolith struct {
	StorageDirectory string
	listener         net.Listener
}

func (m *DendriteMonolith) BaseURL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *DendriteMonolith) Start() {
	logger := logrus.Logger{
		Out: BindLogger{},
	}
	logrus.SetOutput(BindLogger{})

	var err error
	m.listener, err = net.Listen("tcp", "localhost:65432")
	if err != nil {
		panic(err)
	}

	ygg, err := yggconn.Setup("dendrite", "", m.StorageDirectory)
	if err != nil {
		panic(err)
	}

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
	cfg.Database.PublicRoomsAPI = config.DataSource(fmt.Sprintf("file:%s/dendrite-publicroomsa.db", m.StorageDirectory))
	cfg.Database.Naffka = config.DataSource(fmt.Sprintf("file:%s/dendrite-naffka.db", m.StorageDirectory))
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

	publicRoomsDB, err := storage.NewPublicRoomsServerDatabase(string(base.Cfg.Database.PublicRoomsAPI), base.Cfg.DbProperties(), cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to public rooms db")
	}

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
		//ServerKeyAPI:        serverKeyAPI,

		PublicRoomsDB: publicRoomsDB,
	}
	monolith.AddAllPublicRoutes(base.PublicAPIMux)

	httputil.SetupHTTPAPI(
		base.BaseMux,
		base.PublicAPIMux,
		base.InternalAPIMux,
		cfg,
		base.UseHTTPAPIs,
	)

	// Build both ends of a HTTP multiplex.
	httpServer := &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
		Handler: base.BaseMux,
	}

	go func() {
		logger.Info("Listening on ", ygg.DerivedServerName())
		logger.Fatal(httpServer.Serve(ygg))
	}()
	go func() {
		logger.Info("Listening on ", m.BaseURL())
		logger.Fatal(httpServer.Serve(m.listener))
	}()
}
