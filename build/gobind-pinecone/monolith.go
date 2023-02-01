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
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conduit"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/conn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/monolith"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/relay"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/rooms"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/users"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/relayapi"
	relayServerAPI "github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi"
	userapiAPI "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/pinecone/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeEvents "github.com/matrix-org/pinecone/router/events"

	_ "golang.org/x/mobile/bind"
)

const (
	PeerTypeRemote    = pineconeRouter.PeerTypeRemote
	PeerTypeMulticast = pineconeRouter.PeerTypeMulticast
	PeerTypeBluetooth = pineconeRouter.PeerTypeBluetooth
	PeerTypeBonjour   = pineconeRouter.PeerTypeBonjour
)

type DendriteMonolith struct {
	logger           logrus.Logger
	baseDendrite     *base.BaseDendrite
	p2pMonolith      monolith.P2PMonolith
	StorageDirectory string
	listener         net.Listener
	httpServer       *http.Server
	userAPI          userapiAPI.UserInternalAPI
	federationAPI    api.FederationInternalAPI
	relayAPI         relayServerAPI.RelayInternalAPI
	relayRetriever   relay.RelayServerRetriever
}

func (m *DendriteMonolith) PublicKey() string {
	return m.p2pMonolith.Router.PublicKey().String()
}

func (m *DendriteMonolith) BaseURL() string {
	return fmt.Sprintf("http://%s", m.listener.Addr().String())
}

func (m *DendriteMonolith) PeerCount(peertype int) int {
	return m.p2pMonolith.Router.PeerCount(peertype)
}

func (m *DendriteMonolith) SessionCount() int {
	return len(m.p2pMonolith.Sessions.Protocol("matrix").Sessions())
}

type InterfaceInfo struct {
	Name         string
	Index        int
	Mtu          int
	Up           bool
	Broadcast    bool
	Loopback     bool
	PointToPoint bool
	Multicast    bool
	Addrs        string
}

type InterfaceRetriever interface {
	CacheCurrentInterfaces() int
	GetCachedInterface(index int) *InterfaceInfo
}

func (m *DendriteMonolith) RegisterNetworkCallback(intfCallback InterfaceRetriever) {
	callback := func() []pineconeMulticast.InterfaceInfo {
		count := intfCallback.CacheCurrentInterfaces()
		intfs := []pineconeMulticast.InterfaceInfo{}
		for i := 0; i < count; i++ {
			iface := intfCallback.GetCachedInterface(i)
			if iface != nil {
				intfs = append(intfs, pineconeMulticast.InterfaceInfo{
					Name:         iface.Name,
					Index:        iface.Index,
					Mtu:          iface.Mtu,
					Up:           iface.Up,
					Broadcast:    iface.Broadcast,
					Loopback:     iface.Loopback,
					PointToPoint: iface.PointToPoint,
					Multicast:    iface.Multicast,
					Addrs:        iface.Addrs,
				})
			}
		}
		return intfs
	}
	m.p2pMonolith.Multicast.RegisterNetworkCallback(callback)
}

func (m *DendriteMonolith) SetMulticastEnabled(enabled bool) {
	if enabled {
		m.p2pMonolith.Multicast.Start()
	} else {
		m.p2pMonolith.Multicast.Stop()
		m.DisconnectType(int(pineconeRouter.PeerTypeMulticast))
	}
}

func (m *DendriteMonolith) SetStaticPeer(uri string) {
	m.p2pMonolith.ConnManager.RemovePeers()
	for _, uri := range strings.Split(uri, ",") {
		m.p2pMonolith.ConnManager.AddPeer(strings.TrimSpace(uri))
	}
}

func getServerKeyFromString(nodeID string) (gomatrixserverlib.ServerName, error) {
	var nodeKey gomatrixserverlib.ServerName
	if userID, err := gomatrixserverlib.NewUserID(nodeID, false); err == nil {
		hexKey, decodeErr := hex.DecodeString(string(userID.Domain()))
		if decodeErr != nil || len(hexKey) != ed25519.PublicKeySize {
			return "", fmt.Errorf("UserID domain is not a valid ed25519 public key: %v", userID.Domain())
		} else {
			nodeKey = userID.Domain()
		}
	} else {
		hexKey, decodeErr := hex.DecodeString(nodeID)
		if decodeErr != nil || len(hexKey) != ed25519.PublicKeySize {
			return "", fmt.Errorf("Relay server uri is not a valid ed25519 public key: %v", nodeID)
		} else {
			nodeKey = gomatrixserverlib.ServerName(nodeID)
		}
	}

	return nodeKey, nil
}

func (m *DendriteMonolith) SetRelayServers(nodeID string, uris string) {
	relays := []gomatrixserverlib.ServerName{}
	for _, uri := range strings.Split(uris, ",") {
		uri = strings.TrimSpace(uri)
		if len(uri) == 0 {
			continue
		}

		nodeKey, err := getServerKeyFromString(uri)
		if err != nil {
			logrus.Errorf(err.Error())
			continue
		}
		relays = append(relays, nodeKey)
	}

	nodeKey, err := getServerKeyFromString(nodeID)
	if err != nil {
		logrus.Errorf(err.Error())
		return
	}

	if string(nodeKey) == m.PublicKey() {
		logrus.Infof("Setting own relay servers to: %v", relays)
		m.relayRetriever.SetRelayServers(relays)
	} else {
		relay.UpdateNodeRelayServers(
			gomatrixserverlib.ServerName(nodeKey),
			relays,
			m.baseDendrite.Context(),
			m.federationAPI,
		)
	}
}

func (m *DendriteMonolith) GetRelayServers(nodeID string) string {
	nodeKey, err := getServerKeyFromString(nodeID)
	if err != nil {
		logrus.Errorf(err.Error())
		return ""
	}

	relaysString := ""
	if string(nodeKey) == m.PublicKey() {
		relays := m.relayRetriever.GetRelayServers()

		for i, relay := range relays {
			if i != 0 {
				// Append a comma to the previous entry if there is one.
				relaysString += ","
			}
			relaysString += string(relay)
		}
	} else {
		request := api.P2PQueryRelayServersRequest{Server: gomatrixserverlib.ServerName(nodeKey)}
		response := api.P2PQueryRelayServersResponse{}
		err := m.federationAPI.P2PQueryRelayServers(m.baseDendrite.Context(), &request, &response)
		if err != nil {
			logrus.Warnf("Failed obtaining list of this node's relay servers: %s", err.Error())
			return ""
		}

		for i, relay := range response.RelayServers {
			if i != 0 {
				// Append a comma to the previous entry if there is one.
				relaysString += ","
			}
			relaysString += string(relay)
		}
	}

	return relaysString
}

func (m *DendriteMonolith) RelayingEnabled() bool {
	return m.relayAPI.RelayingEnabled()
}

func (m *DendriteMonolith) SetRelayingEnabled(enabled bool) {
	m.relayAPI.SetRelayingEnabled(enabled)
}

func (m *DendriteMonolith) DisconnectType(peertype int) {
	for _, p := range m.p2pMonolith.Router.Peers() {
		if int(peertype) == p.PeerType {
			m.p2pMonolith.Router.Disconnect(types.SwitchPortID(p.Port), nil)
		}
	}
}

func (m *DendriteMonolith) DisconnectZone(zone string) {
	for _, p := range m.p2pMonolith.Router.Peers() {
		if zone == p.Zone {
			m.p2pMonolith.Router.Disconnect(types.SwitchPortID(p.Port), nil)
		}
	}
}

func (m *DendriteMonolith) DisconnectPort(port int) {
	m.p2pMonolith.Router.Disconnect(types.SwitchPortID(port), nil)
}

func (m *DendriteMonolith) Conduit(zone string, peertype int) (*conduit.Conduit, error) {
	l, r := net.Pipe()
	newConduit := conduit.NewConduit(r, 0)
	go func() {
		logrus.Errorf("Attempting authenticated connect")
		var port types.SwitchPortID
		var err error
		if port, err = m.p2pMonolith.Router.Connect(
			l,
			pineconeRouter.ConnectionZone(zone),
			pineconeRouter.ConnectionPeerType(peertype),
		); err != nil {
			logrus.Errorf("Authenticated connect failed: %s", err)
			_ = l.Close()
			_ = r.Close()
			_ = newConduit.Close()
			return
		}
		newConduit.SetPort(port)
		logrus.Infof("Authenticated connect succeeded (port %d)", newConduit.Port())
	}()
	return &newConduit, nil
}

func (m *DendriteMonolith) RegisterUser(localpart, password string) (string, error) {
	pubkey := m.p2pMonolith.Router.PublicKey()
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

func (m *DendriteMonolith) Start() {
	keyfile := filepath.Join(m.StorageDirectory, "p2p.pem")
	oldKeyfile := filepath.Join(m.StorageDirectory, "p2p.key")
	sk, pk := monolith.GetOrCreateKey(keyfile, oldKeyfile)

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

	m.p2pMonolith = monolith.P2PMonolith{}
	m.p2pMonolith.SetupPinecone(sk)

	prefix := hex.EncodeToString(pk)
	cfg := monolith.GenerateDefaultConfig(sk, m.StorageDirectory, prefix)
	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Global.JetStream.InMemory = false

	base := base.NewBaseDendrite(cfg, "Monolith", base.DisableMetrics)
	m.baseDendrite = base
	base.ConfigureAdminEndpoints()

	federation := conn.CreateFederationClient(base, m.p2pMonolith.Sessions)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	rsAPI := roomserver.NewInternalAPI(base)

	m.federationAPI = federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, keyRing, true,
	)

	keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, m.federationAPI, rsAPI)
	m.userAPI = userapi.NewInternalAPI(base, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI, rsAPI, base.PushGatewayHTTPClient())
	keyAPI.SetUserAPI(m.userAPI)

	asAPI := appservice.NewInternalAPI(base, m.userAPI, rsAPI)

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsAPI.SetFederationAPI(m.federationAPI, keyRing)

	userProvider := users.NewPineconeUserProvider(m.p2pMonolith.Router, m.p2pMonolith.Sessions, m.userAPI, federation)
	roomProvider := rooms.NewPineconeRoomProvider(m.p2pMonolith.Router, m.p2pMonolith.Sessions, m.federationAPI, federation)

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
		UserAPI:                m.userAPI,
	}
	m.relayAPI = relayapi.NewRelayInternalAPI(base, federation, rsAPI, keyRing, producer, false)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		Client:    conn.CreateClient(base, m.p2pMonolith.Sessions),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:            asAPI,
		FederationAPI:            m.federationAPI,
		RoomserverAPI:            rsAPI,
		UserAPI:                  m.userAPI,
		KeyAPI:                   keyAPI,
		RelayAPI:                 m.relayAPI,
		ExtPublicRoomsProvider:   roomProvider,
		ExtUserDirectoryProvider: userProvider,
	}
	monolith.AddAllPublicRoutes(base)

	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)
	httpRouter.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(base.DendriteAdminMux)
	httpRouter.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(base.SynapseAdminMux)
	httpRouter.HandleFunc("/pinecone", m.p2pMonolith.Router.ManholeHandler)

	pMux := mux.NewRouter().SkipClean(true).UseEncodedPath()
	pMux.PathPrefix(users.PublicURL).HandlerFunc(userProvider.FederatedUserProfiles)
	pMux.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	pMux.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	pHTTP := m.p2pMonolith.Sessions.Protocol("matrix").HTTP()
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

	go func() {
		m.logger.Info("Listening on ", cfg.Global.ServerName)

		switch m.httpServer.Serve(m.p2pMonolith.Sessions.Protocol("matrix")) {
		case net.ErrClosed, http.ErrServerClosed:
			m.logger.Info("Stopped listening on ", cfg.Global.ServerName)
		default:
			m.logger.Error("Stopped listening on ", cfg.Global.ServerName)
		}
	}()
	go func() {
		logrus.Info("Listening on ", m.listener.Addr())

		switch http.Serve(m.listener, httpRouter) {
		case net.ErrClosed, http.ErrServerClosed:
			m.logger.Info("Stopped listening on ", cfg.Global.ServerName)
		default:
			m.logger.Error("Stopped listening on ", cfg.Global.ServerName)
		}
	}()

	stopRelayServerSync := make(chan bool)
	eLog := logrus.WithField("pinecone", "events")
	m.relayRetriever = relay.NewRelayServerRetriever(
		context.Background(),
		gomatrixserverlib.ServerName(m.p2pMonolith.Router.PublicKey().String()),
		m.federationAPI,
		monolith.RelayAPI,
		stopRelayServerSync,
	)
	m.relayRetriever.InitializeRelayServers(eLog)

	go func(ch <-chan pineconeEvents.Event) {
		for event := range ch {
			switch e := event.(type) {
			case pineconeEvents.PeerAdded:
				m.relayRetriever.StartSync()
			case pineconeEvents.PeerRemoved:
				if m.relayRetriever.IsRunning() && m.p2pMonolith.Router.TotalPeerCount() == 0 {
					stopRelayServerSync <- true
				}
			case pineconeEvents.BroadcastReceived:
				// eLog.Info("Broadcast received from: ", e.PeerID)

				req := &api.PerformWakeupServersRequest{
					ServerNames: []gomatrixserverlib.ServerName{gomatrixserverlib.ServerName(e.PeerID)},
				}
				res := &api.PerformWakeupServersResponse{}
				if err := m.federationAPI.PerformWakeupServers(base.Context(), req, res); err != nil {
					eLog.WithError(err).Error("Failed to wakeup destination", e.PeerID)
				}
			default:
			}
		}
	}(m.p2pMonolith.EventChannel)
}

func (m *DendriteMonolith) Stop() {
	_ = m.baseDendrite.Close()
	m.baseDendrite.WaitForShutdown()
	_ = m.listener.Close()
	m.p2pMonolith.Multicast.Stop()
	_ = m.p2pMonolith.Sessions.Close()
	_ = m.p2pMonolith.Router.Close()
}
