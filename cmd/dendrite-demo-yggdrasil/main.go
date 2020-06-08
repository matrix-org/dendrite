// Copyright 2017 Vector Creations Ltd
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

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/transactions"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/mediaapi"
	"github.com/matrix-org/dendrite/publicroomsapi"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/serverkeyapi"
	"github.com/matrix-org/dendrite/syncapi"
	"github.com/matrix-org/gomatrixserverlib"

	yggdrasiladmin "github.com/yggdrasil-network/yggdrasil-go/src/admin"
	yggdrasilconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
	yggdrasilmulticast "github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	"github.com/yggdrasil-network/yggdrasil-go/src/yggdrasil"

	gologme "github.com/gologme/log"
	"github.com/sirupsen/logrus"
)

var (
	instanceName = flag.String("name", "dendrite-p2p", "the name of this P2P demo instance")
	instancePort = flag.Int("port", 8080, "the port that the client API will listen on")
)

type node struct {
	core      yggdrasil.Core
	config    *yggdrasilconfig.NodeConfig
	state     *yggdrasilconfig.NodeState
	admin     *yggdrasiladmin.AdminSocket
	multicast *yggdrasilmulticast.Multicast
	log       *gologme.Logger
	listener  *yggdrasil.Listener
	dialer    *yggdrasil.Dialer
}

type yggroundtripper struct {
	inner *http.Transport
}

func (y *yggroundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return y.inner.RoundTrip(req)
}

// nolint:gocyclo
func main() {
	flag.Parse()

	// Build an Yggdrasil node.
	n := node{
		config: yggdrasilconfig.GenerateConfig(),
		log:    gologme.New(os.Stdout, "YGG ", log.Flags()),
	}
	//n.config.AdminListen = fmt.Sprintf("unix:%s-yggdrasil.sock", *instanceName)

	yggfile := fmt.Sprintf("%s-yggdrasil.conf", *instanceName)
	if _, err := os.Stat(yggfile); !os.IsNotExist(err) {
		yggconf, e := ioutil.ReadFile(yggfile)
		if e != nil {
			panic(err)
		}
		if err := json.Unmarshal([]byte(yggconf), &n.config); err != nil {
			panic(err)
		}
	} else {
		j, err := json.Marshal(n.config)
		if err != nil {
			panic(err)
		}
		if e := ioutil.WriteFile(yggfile, j, 0600); e != nil {
			fmt.Printf("Couldn't write private key to file '%s': %s\n", yggfile, e)
		}
	}

	var err error
	n.log.EnableLevel("error")
	n.log.EnableLevel("warn")
	n.log.EnableLevel("info")
	n.log.EnableLevel("debug")
	n.state, err = n.core.Start(n.config, n.log)
	if err != nil {
		panic(err)
	}
	_ = n.admin.Init(&n.core, n.state, n.log, nil)
	if err = n.admin.Start(); err != nil {
		panic(err)
	}
	n.admin.SetupAdminHandlers(n.admin)
	n.multicast = &yggdrasilmulticast.Multicast{}
	if err = n.multicast.Init(&n.core, n.state, n.log, nil); err != nil {
		panic(err)
	}
	n.multicast.SetupAdminHandlers(n.admin)
	if err = n.multicast.Start(); err != nil {
		panic(err)
	}
	n.listener, err = n.core.ConnListen()
	if err != nil {
		panic(err)
	}
	n.dialer, err = n.core.ConnDialer()
	if err != nil {
		panic(err)
	}
	yggdialer := func(_, address string) (net.Conn, error) {
		tokens := strings.Split(address, ":")
		return n.dialer.Dial("curve25519", tokens[0])
	}
	yggdialerctx := func(ctx context.Context, network, address string) (net.Conn, error) {
		return yggdialer(network, address)
	}

	// Build both ends of a HTTP multiplex.
	httpServer := &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}
	httpClient := &http.Client{
		Transport: &yggroundtripper{
			inner: &http.Transport{
				DialContext:     yggdialerctx,
				MaxConnsPerHost: 1,
			},
		},
	}

	privBytes, _ := hex.DecodeString(n.config.SigningPrivateKey)
	privKey := ed25519.PrivateKey(privBytes)

	cfg := &config.Dendrite{}
	cfg.Matrix.ServerName = gomatrixserverlib.ServerName(n.core.EncryptionPublicKey())
	cfg.Matrix.PrivateKey = privKey
	cfg.Matrix.KeyID = gomatrixserverlib.KeyID("ed25519:auto")
	cfg.Kafka.UseNaffka = true
	cfg.Kafka.Topics.OutputRoomEvent = "roomserverOutput"
	cfg.Kafka.Topics.OutputClientData = "clientapiOutput"
	cfg.Kafka.Topics.OutputTypingEvent = "typingServerOutput"
	cfg.Kafka.Topics.UserUpdates = "userUpdates"
	cfg.Database.Account = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.Database.Device = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.Database.MediaAPI = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.Database.SyncAPI = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.Database.RoomServer = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.Database.ServerKey = config.DataSource(fmt.Sprintf("file:%s-serverkey.db", *instanceName))
	cfg.Database.FederationSender = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", *instanceName))
	cfg.Database.AppService = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.Database.PublicRoomsAPI = config.DataSource(fmt.Sprintf("file:%s-publicroomsa.db", *instanceName))
	cfg.Database.Naffka = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := basecomponent.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	federation := gomatrixserverlib.NewFederationClientWithHTTPClient(
		cfg.Matrix.ServerName,
		cfg.Matrix.KeyID,
		cfg.Matrix.PrivateKey,
		httpClient,
	)

	serverKeyAPI := serverkeyapi.NewInternalAPI(
		base.Cfg, federation, base.Caches,
	)
	if base.UseHTTPAPIs {
		serverkeyapi.AddInternalRoutes(base.InternalAPIMux, serverKeyAPI, base.Caches)
		serverKeyAPI = base.ServerKeyAPIClient()
	}
	keyRing := serverKeyAPI.KeyRing()

	rsComponent := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)
	rsAPI := rsComponent
	if base.UseHTTPAPIs {
		roomserver.AddInternalRoutes(base.InternalAPIMux, rsAPI)
		rsAPI = base.RoomserverHTTPClient()
	}

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), deviceDB,
	)
	if base.UseHTTPAPIs {
		eduserver.AddInternalRoutes(base.InternalAPIMux, eduInputAPI)
		eduInputAPI = base.EDUServerClient()
	}

	asAPI := appservice.NewInternalAPI(base, accountDB, deviceDB, rsAPI)
	appservice.AddPublicRoutes(base.PublicAPIMux, cfg, rsAPI, accountDB, federation, transactions.New())
	if base.UseHTTPAPIs {
		appservice.AddInternalRoutes(base.InternalAPIMux, asAPI)
		asAPI = base.AppserviceHTTPClient()
	}

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)
	if base.UseHTTPAPIs {
		federationsender.AddInternalRoutes(base.InternalAPIMux, fsAPI)
		fsAPI = base.FederationSenderHTTPClient()
	}
	rsComponent.SetFederationSenderAPI(fsAPI)

	clientapi.AddPublicRoutes(
		base.PublicAPIMux, base, deviceDB, accountDB,
		federation, keyRing, rsAPI,
		eduInputAPI, asAPI, transactions.New(), fsAPI,
	)

	keyserver.AddPublicRoutes(
		base.PublicAPIMux, base.Cfg, deviceDB, accountDB,
	)
	eduProducer := producers.NewEDUServerProducer(eduInputAPI)
	federationapi.AddPublicRoutes(base.PublicAPIMux, base.Cfg, accountDB, deviceDB, federation, keyRing, rsAPI, asAPI, fsAPI, eduProducer)
	mediaapi.AddPublicRoutes(base.PublicAPIMux, base.Cfg, deviceDB)
	publicRoomsDB, err := storage.NewPublicRoomsServerDatabase(string(base.Cfg.Database.PublicRoomsAPI), base.Cfg.DbProperties(), cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to public rooms db")
	}
	publicroomsapi.AddPublicRoutes(base.PublicAPIMux, base, deviceDB, publicRoomsDB, rsAPI, federation, nil)
	syncapi.AddPublicRoutes(base.PublicAPIMux, base, deviceDB, accountDB, rsAPI, federation, cfg)

	internal.SetupHTTPAPI(
		http.DefaultServeMux,
		base.PublicAPIMux,
		base.InternalAPIMux,
		cfg,
		base.UseHTTPAPIs,
	)

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		logrus.Info("Listening on ", n.core.EncryptionPublicKey())
		logrus.Fatal(httpServer.ListenAndServe())
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, nil))
	}()

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}
