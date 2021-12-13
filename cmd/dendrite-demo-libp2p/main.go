// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"crypto/ed25519"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	gostream "github.com/libp2p/go-libp2p-gostream"
	p2phttp "github.com/libp2p/go-libp2p-http"
	p2pdisc "github.com/libp2p/go-libp2p/p2p/discovery"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/embed"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/eduserver/cache"

	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
)

func createKeyDB(
	base *P2PDendrite,
	db *gomatrixserverlib.KeyRing,
) {
	mdns := mDNSListener{
		host:  base.LibP2P,
		keydb: db,
	}
	serv, err := p2pdisc.NewMdnsService(
		base.LibP2PContext,
		base.LibP2P,
		time.Second*10,
		"_matrix-dendrite-p2p._tcp",
	)
	if err != nil {
		panic(err)
	}
	serv.RegisterNotifee(&mdns)
}

func createFederationClient(
	base *P2PDendrite,
) *gomatrixserverlib.FederationClient {
	fmt.Println("Running in libp2p federation mode")
	fmt.Println("Warning: Federation with non-libp2p homeservers will not work in this mode yet!")
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix",
		p2phttp.NewTransport(base.LibP2P, p2phttp.ProtocolOption("/matrix")),
	)
	return gomatrixserverlib.NewFederationClient(
		base.Base.Cfg.Global.ServerName, base.Base.Cfg.Global.KeyID,
		base.Base.Cfg.Global.PrivateKey,
		gomatrixserverlib.WithTransport(tr),
	)
}

func createClient(
	base *P2PDendrite,
) *gomatrixserverlib.Client {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix",
		p2phttp.NewTransport(base.LibP2P, p2phttp.ProtocolOption("/matrix")),
	)
	return gomatrixserverlib.NewClient(
		gomatrixserverlib.WithTransport(tr),
	)
}

func main() {
	instanceName := flag.String("name", "dendrite-p2p", "the name of this P2P demo instance")
	instancePort := flag.Int("port", 8080, "the port that the client API will listen on")
	flag.Parse()

	filename := fmt.Sprintf("%s-private.key", *instanceName)
	_, err := os.Stat(filename)
	var privKey ed25519.PrivateKey
	if os.IsNotExist(err) {
		_, privKey, _ = ed25519.GenerateKey(nil)
		if err = ioutil.WriteFile(filename, privKey, 0600); err != nil {
			fmt.Printf("Couldn't write private key to file '%s': %s\n", filename, err)
		}
	} else {
		privKey, err = ioutil.ReadFile(filename)
		if err != nil {
			fmt.Printf("Couldn't read private key from file '%s': %s\n", filename, err)
			_, privKey, _ = ed25519.GenerateKey(nil)
		}
	}

	cfg := config.Dendrite{}
	cfg.Defaults(true)
	cfg.Global.ServerName = "p2p"
	cfg.Global.PrivateKey = privKey
	cfg.Global.KeyID = gomatrixserverlib.KeyID(fmt.Sprintf("ed25519:%s", *instanceName))
	cfg.Global.Kafka.UseNaffka = true
	cfg.FederationAPI.FederationMaxRetries = 6
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationapi.db", *instanceName))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.Global.Kafka.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-e2ekey.db", *instanceName))
	cfg.MSCs.MSCs = []string{"msc2836"}
	cfg.MSCs.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mscs.db", *instanceName))
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := NewP2PDendrite(&cfg, "Monolith")
	defer base.Base.Close() // nolint: errcheck

	accountDB := base.Base.CreateAccountsDB()
	federation := createFederationClient(base)
	keyAPI := keyserver.NewInternalAPI(&base.Base, &base.Base.Cfg.KeyServer, federation)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, nil, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	rsAPI := roomserver.NewInternalAPI(
		&base.Base,
	)
	eduInputAPI := eduserver.NewInternalAPI(
		&base.Base, cache.New(), userAPI,
	)
	asAPI := appservice.NewInternalAPI(&base.Base, userAPI, rsAPI)
	rsAPI.SetAppserviceAPI(asAPI)
	fsAPI := federationapi.NewInternalAPI(
		&base.Base, federation, rsAPI, base.Base.Caches, nil, true,
	)
	keyRing := fsAPI.KeyRing()
	rsAPI.SetFederationAPI(fsAPI, keyRing)
	provider := newPublicRoomsProvider(base.LibP2PPubsub, rsAPI)
	err = provider.Start()
	if err != nil {
		panic("failed to create new public rooms provider: " + err.Error())
	}

	createKeyDB(
		base, keyRing,
	)

	monolith := setup.Monolith{
		Config:    base.Base.Cfg,
		AccountDB: accountDB,
		Client:    createClient(base),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:          asAPI,
		EDUInternalAPI:         eduInputAPI,
		FederationAPI:          fsAPI,
		RoomserverAPI:          rsAPI,
		UserAPI:                userAPI,
		KeyAPI:                 keyAPI,
		ExtPublicRoomsProvider: provider,
	}
	monolith.AddAllPublicRoutes(
		base.Base.ProcessContext,
		base.Base.PublicClientAPIMux,
		base.Base.PublicFederationAPIMux,
		base.Base.PublicKeyAPIMux,
		base.Base.PublicWellKnownAPIMux,
		base.Base.PublicMediaAPIMux,
		base.Base.SynapseAdminMux,
	)
	if err := mscs.Enable(&base.Base, &monolith); err != nil {
		logrus.WithError(err).Fatalf("Failed to enable MSCs")
	}

	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.Base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.Base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.Base.PublicMediaAPIMux)
	embed.Embed(httpRouter, *instancePort, "Yggdrasil Demo")

	libp2pRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	libp2pRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.Base.PublicFederationAPIMux)
	libp2pRouter.PathPrefix(httputil.PublicKeyPathPrefix).Handler(base.Base.PublicKeyAPIMux)
	libp2pRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.Base.PublicMediaAPIMux)

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, httpRouter))
	}()
	// Expose the matrix APIs also via libp2p
	if base.LibP2P != nil {
		go func() {
			logrus.Info("Listening on libp2p host ID ", base.LibP2P.ID())
			listener, err := gostream.Listen(base.LibP2P, "/matrix")
			if err != nil {
				panic(err)
			}
			defer func() {
				logrus.Fatal(listener.Close())
			}()
			logrus.Fatal(http.Serve(listener, libp2pRouter))
		}()
	}

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	base.Base.WaitForShutdown()
}
