package gobind

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/hjson/hjson-go"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/rooms"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/userapi"
	userapiAPI "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"

	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
	"github.com/matrix-org/pinecone/types"
	yggdrasilConfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
)

const (
	PeerTypeRemote    = pineconeRouter.PeerTypeRemote
	PeerTypeMulticast = pineconeRouter.PeerTypeMulticast
	PeerTypeBluetooth = pineconeRouter.PeerTypeBluetooth
)

type DendriteMonolith struct {
	logger            logrus.Logger
	config            *yggdrasilConfig.NodeConfig
	PineconeRouter    *pineconeRouter.Router
	PineconeMulticast *pineconeMulticast.Multicast
	PineconeQUIC      *pineconeSessions.QUIC
	StorageDirectory  string
	listener          net.Listener
	httpServer        *http.Server
	processContext    *process.ProcessContext
	userAPI           userapiAPI.UserInternalAPI
}

func (m *DendriteMonolith) BaseURL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *DendriteMonolith) PeerCount() int {
	return m.PineconeRouter.PeerCount()
}

func (m *DendriteMonolith) SessionCount() int {
	return len(m.PineconeQUIC.Sessions())
}

func (m *DendriteMonolith) SetMulticastEnabled(enabled bool) {
	if enabled {
		m.PineconeMulticast.Start()
	} else {
		m.PineconeMulticast.Stop()
		m.DisconnectType(pineconeRouter.PeerTypeMulticast)
	}
}

func (m *DendriteMonolith) SetStaticPeer(uri string) {
	m.DisconnectType(pineconeRouter.PeerTypeRemote)
	if uri != "" {
		go conn.ConnectToPeer(m.PineconeRouter, uri)
	}
}

func (m *DendriteMonolith) DisconnectType(peertype int) {
	for _, p := range m.PineconeRouter.Peers() {
		if peertype == p.PeerType {
			_ = m.PineconeRouter.Disconnect(types.SwitchPortID(p.Port))
		}
	}
}

func (m *DendriteMonolith) DisconnectZone(zone string) {
	for _, p := range m.PineconeRouter.Peers() {
		if zone == p.Zone {
			_ = m.PineconeRouter.Disconnect(types.SwitchPortID(p.Port))
		}
	}
}

func (m *DendriteMonolith) DisconnectPort(port int) error {
	return m.PineconeRouter.Disconnect(types.SwitchPortID(port))
}

func (m *DendriteMonolith) Conduit(zone string, peertype int) (*Conduit, error) {
	l, r := net.Pipe()
	conduit := &Conduit{conn: r, port: 0}
	go func() {
		conduit.portMutex.Lock()
		defer conduit.portMutex.Unlock()
	loop:
		for i := 1; i <= 10; i++ {
			logrus.Errorf("Attempting authenticated connect (attempt %d)", i)
			var err error
			conduit.port, err = m.PineconeRouter.AuthenticatedConnect(l, zone, peertype)
			switch err {
			case io.ErrClosedPipe:
				logrus.Errorf("Authenticated connect failed due to closed pipe (attempt %d)", i)
				return
			case io.EOF:
				logrus.Errorf("Authenticated connect failed due to EOF (attempt %d)", i)
				break loop
			case nil:
				logrus.Errorf("Authenticated connect succeeded, connected to port %d (attempt %d)", conduit.port, i)
				return
			default:
				logrus.WithError(err).Errorf("Authenticated connect failed (attempt %d)", i)
				time.Sleep(time.Second)
			}
		}
		_ = l.Close()
		_ = r.Close()
	}()
	return conduit, nil
}

func (m *DendriteMonolith) RegisterUser(localpart, password string) (string, error) {
	pubkey := m.PineconeRouter.PublicKey()
	userID := userutil.MakeUserID(
		localpart,
		gomatrixserverlib.ServerName(hex.EncodeToString(pubkey[:])),
	)
	userReq := &userapiAPI.PerformAccountCreationRequest{
		AccountType: userapiAPI.AccountTypeUser,
		Localpart:   localpart,
		Password:    password,
	}
	userRes := &userapiAPI.PerformAccountCreationResponse{}
	if err := m.userAPI.PerformAccountCreation(context.Background(), userReq, userRes); err != nil {
		return userID, fmt.Errorf("userAPI.PerformAccountCreation: %w", err)
	}
	return userID, nil
}

func (m *DendriteMonolith) RegisterDevice(localpart, deviceID string) (string, error) {
	accessTokenBytes := make([]byte, 16)
	n, err := rand.Read(accessTokenBytes)
	if err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	loginReq := &userapiAPI.PerformDeviceCreationRequest{
		Localpart:   localpart,
		DeviceID:    &deviceID,
		AccessToken: hex.EncodeToString(accessTokenBytes[:n]),
	}
	loginRes := &userapiAPI.PerformDeviceCreationResponse{}
	if err := m.userAPI.PerformDeviceCreation(context.Background(), loginReq, loginRes); err != nil {
		return "", fmt.Errorf("userAPI.PerformDeviceCreation: %w", err)
	}
	if !loginRes.DeviceCreated {
		return "", fmt.Errorf("device was not created")
	}
	return loginRes.Device.AccessToken, nil
}

// nolint:gocyclo
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

	logger := log.New(os.Stdout, "PINECONE: ", 0)
	m.PineconeRouter = pineconeRouter.NewRouter(logger, "dendrite", sk, pk, nil)
	m.PineconeQUIC = pineconeSessions.NewQUIC(logger, m.PineconeRouter)
	m.PineconeMulticast = pineconeMulticast.NewMulticast(logger, m.PineconeRouter)

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

	rsAPI := roomserver.NewInternalAPI(
		base, keyRing,
	)

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)

	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, fsAPI)
	m.userAPI = userapi.NewInternalAPI(accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI)
	keyAPI.SetUserAPI(m.userAPI)

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), m.userAPI,
	)

	asAPI := appservice.NewInternalAPI(base, m.userAPI, rsAPI)

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
		UserAPI:                m.userAPI,
		KeyAPI:                 keyAPI,
		ExtPublicRoomsProvider: rooms.NewPineconeRoomProvider(m.PineconeRouter, m.PineconeQUIC, fsAPI, federation),
	}
	monolith.AddAllPublicRoutes(
		base.ProcessContext,
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
	m.processContext = base.ProcessContext

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

func (m *DendriteMonolith) Stop() {
	_ = m.listener.Close()
	m.PineconeMulticast.Stop()
	_ = m.PineconeQUIC.Close()
	m.processContext.ShutdownDendrite()
	_ = m.PineconeRouter.Close()
}

type Conduit struct {
	conn      net.Conn
	port      types.SwitchPortID
	portMutex sync.Mutex
}

func (c *Conduit) Port() int {
	c.portMutex.Lock()
	defer c.portMutex.Unlock()
	return int(c.port)
}

func (c *Conduit) Read(b []byte) (int, error) {
	return c.conn.Read(b)
}

func (c *Conduit) ReadCopy() ([]byte, error) {
	var buf [65535 * 2]byte
	n, err := c.conn.Read(buf[:])
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (c *Conduit) Write(b []byte) (int, error) {
	return c.conn.Write(b)
}

func (c *Conduit) Close() error {
	return c.conn.Close()
}
