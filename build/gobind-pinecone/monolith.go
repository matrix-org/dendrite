package gobind

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/hjson/hjson-go"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/rooms"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
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

	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeSwitch "github.com/matrix-org/pinecone/packetswitch"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
	yggdrasilConfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
)

type DendriteMonolith struct {
	logger            logrus.Logger
	config            *yggdrasilConfig.NodeConfig
	PineconeSwitch    *pineconeSwitch.Switch
	PineconeRouter    *pineconeRouter.Router
	PineconeMulticast *pineconeMulticast.Multicast
	PineconeQUIC      *pineconeSessions.QUIC
	StorageDirectory  string
	listener          net.Listener
	httpServer        *http.Server
}

func (m *DendriteMonolith) BaseURL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *DendriteMonolith) PeerCount() int {
	return m.PineconeSwitch.PeerCount()
}

func (m *DendriteMonolith) SessionCount() int {
	return len(m.PineconeQUIC.Sessions())
}

func (m *DendriteMonolith) SetMulticastEnabled(enabled bool) {
	// TODO
}

func (m *DendriteMonolith) SetStaticPeer(uri string) error {
	go func() {
		parent, err := net.Dial("tcp", uri)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to connect to Pinecone static peer")
			return
		}

		if _, err := m.PineconeSwitch.AuthenticatedConnect(parent, "static"); err != nil {
			logrus.WithError(err).Errorf("Failed to connect Pinecone static peer to switch")
			return
		}
	}()

	return nil
}

func (m *DendriteMonolith) DisconnectNonMulticastPeers() {
	// TODO
}

func (m *DendriteMonolith) DisconnectMulticastPeers() {
	// TODO
}

func (m *DendriteMonolith) Start() {
	m.config = yggdrasilConfig.GenerateConfig()
	// If we already have an Yggdrasil config file from the Ygg
	// demo then we'll just reuse the ed25519 keys from that.
	var sk ed25519.PrivateKey
	var pk ed25519.PublicKey
	yggfile := fmt.Sprintf("%s/dendrite-yggdrasil.conf", m.StorageDirectory)
	if _, err := os.Stat(yggfile); err == nil {
		yggconf, e := ioutil.ReadFile(yggfile)
		if e != nil {
			panic(err)
		}
		mapconf := map[string]interface{}{}
		if err = hjson.Unmarshal([]byte(yggconf), &mapconf); err != nil {
			panic(err)
		}
		if mapconf["SigningPrivateKey"] != nil && mapconf["SigningPublicKey"] != nil {
			m.config.SigningPrivateKey = mapconf["SigningPrivateKey"].(string)
			m.config.SigningPublicKey = mapconf["SigningPublicKey"].(string)
			if sk, err = hex.DecodeString(m.config.SigningPrivateKey); err != nil {
				panic(err)
			}
			if pk, err = hex.DecodeString(m.config.SigningPublicKey); err != nil {
				panic(err)
			}
		} else {
			panic("should have private and public signing keys")
		}
	} else {
		if pk, sk, err = ed25519.GenerateKey(nil); err != nil {
			panic(err)
		}
		m.config.SigningPrivateKey = hex.EncodeToString(sk)
		m.config.SigningPublicKey = hex.EncodeToString(pk)
		yggconf, err := json.Marshal(m.config)
		if err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(yggfile, yggconf, 0644); err != nil {
			panic(err)
		}
	}

	var err error
	m.listener, err = net.Listen("tcp", "localhost:65432")
	if err != nil {
		panic(err)
	}

	m.logger = logrus.Logger{
		Out: BindLogger{},
	}
	m.logger.SetOutput(BindLogger{})
	logrus.SetOutput(BindLogger{})

	logger := log.New(os.Stdout, "", 0)

	rL, rR := net.Pipe()
	m.PineconeSwitch = pineconeSwitch.NewSwitch(logger, sk, pk, false)
	m.PineconeRouter = pineconeRouter.NewRouter(logger, sk, pk, rL, "router", nil)
	if _, err := m.PineconeSwitch.Connect(rR, nil, ""); err != nil {
		panic(err)
	}

	m.PineconeQUIC = pineconeSessions.NewQUIC(logger, m.PineconeRouter)
	m.PineconeMulticast = pineconeMulticast.NewMulticast(logger, m.PineconeSwitch)

	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.PrivateKey = sk
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.Kafka.UseNaffka = true
	cfg.Global.Kafka.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-naffka.db", m.StorageDirectory))
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-account.db", m.StorageDirectory))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-device.db", m.StorageDirectory))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-mediaapi.db", m.StorageDirectory))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-syncapi.db", m.StorageDirectory))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-roomserver.db", m.StorageDirectory))
	cfg.SigningKeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-signingkeyserver.db", m.StorageDirectory))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-keyserver.db", m.StorageDirectory))
	cfg.FederationSender.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-federationsender.db", m.StorageDirectory))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-appservice.db", m.StorageDirectory))
	cfg.MediaAPI.BasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	cfg.MediaAPI.AbsBasePath = config.Path(fmt.Sprintf("%s/tmp", m.StorageDirectory))
	if err := cfg.Derive(); err != nil {
		panic(err)
	}

	base := setup.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := conn.CreateFederationClient(base, m.PineconeQUIC)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()
	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, federation)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	rsAPI := roomserver.NewInternalAPI(
		base, keyRing,
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

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    conn.CreateClient(base, m.PineconeQUIC),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:          asAPI,
		EDUInternalAPI:         eduInputAPI,
		FederationSenderAPI:    fsAPI,
		RoomserverAPI:          rsAPI,
		UserAPI:                userAPI,
		KeyAPI:                 keyAPI,
		ExtPublicRoomsProvider: rooms.NewPineconeRoomProvider(m.PineconeSwitch, m.PineconeRouter, m.PineconeQUIC, fsAPI, federation),
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

	pMux := mux.NewRouter().SkipClean(true).UseEncodedPath()
	pMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	pMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	m.PineconeQUIC.Mux().Handle(httputil.PublicFederationPathPrefix, pMux)
	m.PineconeQUIC.Mux().Handle(httputil.PublicMediaPathPrefix, pMux)

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
		Handler: pMux,
	}

	go func() {
		m.logger.Info("Listening on ", cfg.Global.ServerName)
		m.logger.Fatal(m.httpServer.Serve(m.PineconeQUIC))
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
