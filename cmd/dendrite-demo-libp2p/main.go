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
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/signingkeyserver"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/eduserver/cache"

	"github.com/sirupsen/logrus"
)

func createKeyDB(
	base *P2PDendrite,
	db gomatrixserverlib.KeyDatabase,
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
	return gomatrixserverlib.NewFederationClientWithTransport(
		base.Base.Cfg.Global.ServerName, base.Base.Cfg.Global.KeyID,
		base.Base.Cfg.Global.PrivateKey, true, tr,
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
	return gomatrixserverlib.NewClientWithTransport(true, tr)
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
	cfg.Defaults()
	cfg.Global.ServerName = "p2p"
	cfg.Global.PrivateKey = privKey
	cfg.Global.KeyID = gomatrixserverlib.KeyID(fmt.Sprintf("ed25519:%s", *instanceName))
	cfg.Global.Kafka.UseNaffka = true
	cfg.FederationSender.FederationMaxRetries = 6
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.SigningKeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-signingkeyserver.db", *instanceName))
	cfg.FederationSender.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", *instanceName))
	cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.Global.Kafka.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-e2ekey.db", *instanceName))
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := NewP2PDendrite(&cfg, "Monolith")
	defer base.Base.Close() // nolint: errcheck

	accountDB := base.Base.CreateAccountsDB()
	federation := createFederationClient(base)
	keyAPI := keyserver.NewInternalAPI(&base.Base.Cfg.KeyServer, federation, base.Base.KafkaProducer)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, nil, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	serverKeyAPI := signingkeyserver.NewInternalAPI(
		&base.Base.Cfg.SigningKeyServer, federation, base.Base.Caches,
	)
	keyRing := serverKeyAPI.KeyRing()
	createKeyDB(
		base, serverKeyAPI,
	)

	rsAPI := roomserver.NewInternalAPI(
		&base.Base, keyRing,
	)
	eduInputAPI := eduserver.NewInternalAPI(
		&base.Base, cache.New(), userAPI,
	)
	asAPI := appservice.NewInternalAPI(&base.Base, userAPI, rsAPI)
	fsAPI := federationsender.NewInternalAPI(
		&base.Base, federation, rsAPI, keyRing,
	)
	rsAPI.SetFederationSenderAPI(fsAPI)
	provider := newPublicRoomsProvider(base.LibP2PPubsub, rsAPI)
	err = provider.Start()
	if err != nil {
		panic("failed to create new public rooms provider: " + err.Error())
	}

	monolith := setup.Monolith{
		Config:        base.Base.Cfg,
		AccountDB:     accountDB,
		Client:        createClient(base),
		FedClient:     federation,
		KeyRing:       keyRing,
		KafkaConsumer: base.Base.KafkaConsumer,
		KafkaProducer: base.Base.KafkaProducer,

		AppserviceAPI:          asAPI,
		EDUInternalAPI:         eduInputAPI,
		FederationSenderAPI:    fsAPI,
		RoomserverAPI:          rsAPI,
		ServerKeyAPI:           serverKeyAPI,
		UserAPI:                userAPI,
		KeyAPI:                 keyAPI,
		ExtPublicRoomsProvider: provider,
	}
	monolith.AddAllPublicRoutes(
		base.Base.PublicClientAPIMux,
		base.Base.PublicFederationAPIMux,
		base.Base.PublicKeyAPIMux,
		base.Base.PublicMediaAPIMux,
	)

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
	select {}
}
