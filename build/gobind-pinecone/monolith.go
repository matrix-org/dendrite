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

package gobind

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/rooms"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/users"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/userapi"
	userapiAPI "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	pineconeConnections "github.com/matrix-org/pinecone/connections"
	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
	"github.com/matrix-org/pinecone/types"

	_ "golang.org/x/mobile/bind"
)

const (
	PeerTypeRemote    = pineconeRouter.PeerTypeRemote
	PeerTypeMulticast = pineconeRouter.PeerTypeMulticast
	PeerTypeBluetooth = pineconeRouter.PeerTypeBluetooth
)

type DendriteMonolith struct {
	logger            logrus.Logger
	PineconeRouter    *pineconeRouter.Router
	PineconeMulticast *pineconeMulticast.Multicast
	PineconeQUIC      *pineconeSessions.Sessions
	PineconeManager   *pineconeConnections.ConnectionManager
	StorageDirectory  string
	CacheDirectory    string
	listener          net.Listener
	httpServer        *http.Server
	processContext    *process.ProcessContext
	userAPI           userapiAPI.UserInternalAPI
}

func (m *DendriteMonolith) BaseURL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *DendriteMonolith) PeerCount(peertype int) int {
	return m.PineconeRouter.PeerCount(peertype)
}

func (m *DendriteMonolith) SessionCount() int {
	return len(m.PineconeQUIC.Protocol("matrix").Sessions())
}

func (m *DendriteMonolith) SetMulticastEnabled(enabled bool) {
	if enabled {
		m.PineconeMulticast.Start()
	} else {
		m.PineconeMulticast.Stop()
		m.DisconnectType(int(pineconeRouter.PeerTypeMulticast))
	}
}

func (m *DendriteMonolith) SetStaticPeer(uri string) {
	m.PineconeManager.RemovePeers()
	m.PineconeManager.AddPeer(strings.TrimSpace(uri))
}

func (m *DendriteMonolith) DisconnectType(peertype int) {
	for _, p := range m.PineconeRouter.Peers() {
		if int(peertype) == p.PeerType {
			m.PineconeRouter.Disconnect(types.SwitchPortID(p.Port), nil)
		}
	}
}

func (m *DendriteMonolith) DisconnectZone(zone string) {
	for _, p := range m.PineconeRouter.Peers() {
		if zone == p.Zone {
			m.PineconeRouter.Disconnect(types.SwitchPortID(p.Port), nil)
		}
	}
}

func (m *DendriteMonolith) DisconnectPort(port int) {
	m.PineconeRouter.Disconnect(types.SwitchPortID(port), nil)
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
			conduit.port, err = m.PineconeRouter.Connect(
				l,
				pineconeRouter.ConnectionZone(zone),
				pineconeRouter.ConnectionPeerType(peertype),
			)
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
	var err error
	var sk ed25519.PrivateKey
	var pk ed25519.PublicKey
	keyfile := fmt.Sprintf("%s/p2p.key", m.StorageDirectory)
	if _, err = os.Stat(keyfile); os.IsNotExist(err) {
		if pk, sk, err = ed25519.GenerateKey(nil); err != nil {
			panic(err)
		}
		if err = ioutil.WriteFile(keyfile, sk, 0644); err != nil {
			panic(err)
		}
	} else if err == nil {
		if sk, err = ioutil.ReadFile(keyfile); err != nil {
			panic(err)
		}
		if len(sk) != ed25519.PrivateKeySize {
			panic("the private key is not long enough")
		}
		pk = sk.Public().(ed25519.PublicKey)
	}

	m.listener, err = net.Listen("tcp", ":65432")
	if err != nil {
		panic(err)
	}

	m.logger = logrus.Logger{
		Out: BindLogger{},
	}
	m.logger.SetOutput(BindLogger{})
	logrus.SetOutput(BindLogger{})

	m.PineconeRouter = pineconeRouter.NewRouter(logrus.WithField("pinecone", "router"), sk, false)
	m.PineconeQUIC = pineconeSessions.NewSessions(logrus.WithField("pinecone", "sessions"), m.PineconeRouter, []string{"matrix"})
	m.PineconeMulticast = pineconeMulticast.NewMulticast(logrus.WithField("pinecone", "multicast"), m.PineconeRouter)
	m.PineconeManager = pineconeConnections.NewConnectionManager(m.PineconeRouter)

	prefix := hex.EncodeToString(pk)
	cfg := &config.Dendrite{}
	cfg.Defaults(true)
	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.PrivateKey = sk
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.JetStream.InMemory = true
	cfg.Global.JetStream.StoragePath = config.Path(fmt.Sprintf("%s/%s", m.StorageDirectory, prefix))
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/%s-account.db", m.StorageDirectory, prefix))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/dendrite-p2p-mediaapi.db", m.StorageDirectory))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/%s-syncapi.db", m.StorageDirectory, prefix))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/%s-roomserver.db", m.StorageDirectory, prefix))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/%s-keyserver.db", m.StorageDirectory, prefix))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/%s-federationsender.db", m.StorageDirectory, prefix))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s/%s-appservice.db", m.StorageDirectory, prefix))
	cfg.MediaAPI.BasePath = config.Path(fmt.Sprintf("%s/media", m.CacheDirectory))
	cfg.MediaAPI.AbsBasePath = config.Path(fmt.Sprintf("%s/media", m.CacheDirectory))
	cfg.MSCs.MSCs = []string{"msc2836", "msc2946"}
	if err := cfg.Derive(); err != nil {
		panic(err)
	}

	base := base.NewBaseDendrite(cfg, "Monolith")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := conn.CreateFederationClient(base, m.PineconeQUIC)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	rsAPI := roomserver.NewInternalAPI(base)

	fsAPI := federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, keyRing, true,
	)

	keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, fsAPI)
	m.userAPI = userapi.NewInternalAPI(base, accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI, rsAPI, base.PushGatewayHTTPClient())
	keyAPI.SetUserAPI(m.userAPI)

	asAPI := appservice.NewInternalAPI(base, m.userAPI, rsAPI)

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsAPI.SetFederationAPI(fsAPI, keyRing)

	userProvider := users.NewPineconeUserProvider(m.PineconeRouter, m.PineconeQUIC, m.userAPI, federation)
	roomProvider := rooms.NewPineconeRoomProvider(m.PineconeRouter, m.PineconeQUIC, fsAPI, federation)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    conn.CreateClient(base, m.PineconeQUIC),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:            asAPI,
		FederationAPI:            fsAPI,
		RoomserverAPI:            rsAPI,
		UserAPI:                  m.userAPI,
		KeyAPI:                   keyAPI,
		ExtPublicRoomsProvider:   roomProvider,
		ExtUserDirectoryProvider: userProvider,
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

	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)
	httpRouter.HandleFunc("/pinecone", m.PineconeRouter.ManholeHandler)

	pMux := mux.NewRouter().SkipClean(true).UseEncodedPath()
	pMux.PathPrefix(users.PublicURL).HandlerFunc(userProvider.FederatedUserProfiles)
	pMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	pMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	pHTTP := m.PineconeQUIC.Protocol("matrix").HTTP()
	pHTTP.Mux().Handle(users.PublicURL, pMux)
	pHTTP.Mux().Handle(httputil.PublicFederationPathPrefix, pMux)
	pHTTP.Mux().Handle(httputil.PublicMediaPathPrefix, pMux)

	// Build both ends of a HTTP multiplex.
	h2s := &http2.Server{}
	m.httpServer = &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
		Handler: h2c.NewHandler(pMux, h2s),
	}

	m.processContext = base.ProcessContext

	go func() {
		m.logger.Info("Listening on ", cfg.Global.ServerName)
		m.logger.Fatal(m.httpServer.Serve(m.PineconeQUIC.Protocol("matrix")))
	}()
	go func() {
		logrus.Info("Listening on ", m.listener.Addr())
		logrus.Fatal(http.Serve(m.listener, httpRouter))
	}()
}

func (m *DendriteMonolith) Stop() {
	m.processContext.ShutdownDendrite()
	_ = m.listener.Close()
	m.PineconeMulticast.Stop()
	_ = m.PineconeQUIC.Close()
	_ = m.PineconeRouter.Close()
	m.processContext.WaitForComponentsToFinish()
}

const MaxFrameSize = types.MaxFrameSize

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
