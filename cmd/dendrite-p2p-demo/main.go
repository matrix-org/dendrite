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
	"crypto/ed25519"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"

	gostream "github.com/libp2p/go-libp2p-gostream"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/mediaapi"
	"github.com/matrix-org/dendrite/publicroomsapi"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/syncapi"

	"github.com/matrix-org/dendrite/eduserver/cache"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

const PrivateKeyFileName = ".dendrite-p2p-private"

func main() {
	filename := PrivateKeyFileName
	if u, err := user.Current(); err == nil {
		filename = fmt.Sprintf("%s/%s", u.HomeDir, PrivateKeyFileName)
	}

	_, err := os.Stat(filename)
	var privKey ed25519.PrivateKey
	if os.IsNotExist(err) {
		_, privKey, _ = ed25519.GenerateKey(nil)
		if err := ioutil.WriteFile(filename, privKey, 0600); err != nil {
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
	cfg.Matrix.ServerName = "p2p"
	cfg.Matrix.PrivateKey = privKey
	cfg.Matrix.KeyID = "ed25519:p2pdemo"
	cfg.Kafka.UseNaffka = true
	cfg.Kafka.Topics.OutputRoomEvent = "roomserverOutput"
	cfg.Kafka.Topics.OutputClientData = "clientapiOutput"
	cfg.Kafka.Topics.OutputTypingEvent = "typingServerOutput"
	cfg.Kafka.Topics.UserUpdates = "userUpdates"
	cfg.Database.Account = config.DataSource("file:account.db")
	cfg.Database.Device = config.DataSource("file:device.db")
	cfg.Database.MediaAPI = config.DataSource("file:media_api.db")
	cfg.Database.SyncAPI = config.DataSource("file:sync_api.db")
	cfg.Database.RoomServer = config.DataSource("file:room_server.db")
	cfg.Database.ServerKey = config.DataSource("file:server_key.db")
	cfg.Database.FederationSender = config.DataSource("file:federation_sender.db")
	cfg.Database.AppService = config.DataSource("file:app_service.db")
	cfg.Database.PublicRoomsAPI = config.DataSource("file:public_rooms_api.db")
	cfg.Database.Naffka = config.DataSource("file:naffka.db")
	cfg.Derive()

	base := basecomponent.NewBaseDendrite(&cfg, "Monolith")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	keyDB := base.CreateKeyDB()
	federation := base.CreateFederationClient()
	keyRing := keydb.CreateKeyRing(federation.Client, keyDB)

	alias, input, query := roomserver.SetupRoomServerComponent(base)
	eduInputAPI := eduserver.SetupEDUServerComponent(base, cache.New())
	asQuery := appservice.SetupAppServiceAPIComponent(
		base, accountDB, deviceDB, federation, alias, query, transactions.New(),
	)
	fedSenderAPI := federationsender.SetupFederationSenderComponent(base, federation, query)

	clientapi.SetupClientAPIComponent(
		base, deviceDB, accountDB,
		federation, &keyRing, alias, input, query,
		eduInputAPI, asQuery, transactions.New(), fedSenderAPI,
	)
	eduProducer := producers.NewEDUServerProducer(eduInputAPI)
	federationapi.SetupFederationAPIComponent(base, accountDB, deviceDB, federation, &keyRing, alias, input, query, asQuery, fedSenderAPI, eduProducer)
	mediaapi.SetupMediaAPIComponent(base, deviceDB)
	publicroomsapi.SetupPublicRoomsAPIComponent(base, deviceDB, query, federation, nil) // Check this later
	syncapi.SetupSyncAPIComponent(base, deviceDB, accountDB, query, federation, &cfg)

	httpHandler := common.WrapHandlerInCORS(base.APIMux)

	// Set up the API endpoints we handle. /metrics is for prometheus, and is
	// not wrapped by CORS, while everything else is
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/", httpHandler)

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		httpBindAddr := ":8080"
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, nil))
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
			logrus.Fatal(http.Serve(listener, nil))
		}()
	}

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}
