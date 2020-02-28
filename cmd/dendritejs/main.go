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
	"flag"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/mediaapi"
	"github.com/matrix-org/dendrite/publicroomsapi"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/syncapi"
	"github.com/matrix-org/dendrite/typingserver"
	"github.com/matrix-org/dendrite/typingserver/cache"
	"github.com/matrix-org/go-http-js-libp2p/go_http_js_libp2p"

	"github.com/sirupsen/logrus"

	_ "github.com/matrix-org/go-sqlite3-js"
)

func init() {
	fmt.Println("dendrite.js starting...")
}

var (
	httpBindAddr  = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	certFile      = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile       = flag.String("tls-key", "", "The PEM private key to use for TLS")
)

func generateKey() ed25519.PrivateKey {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		logrus.Fatalf("Failed to generate ed25519 key: %s", err)
	}
	return priv
}

func main() {
	cfg := &config.Dendrite{}
	cfg.SetDefaults()
	cfg.Kafka.UseNaffka = true
	cfg.Database.Account = "file:dendritejs_account.db?txns=false"
	cfg.Database.AppService = "file:dendritejs_appservice.db?txns=false"
	cfg.Database.Device = "file:dendritejs_device.db?txns=false"
	cfg.Database.FederationSender = "file:dendritejs_fedsender.db?txns=false"
	cfg.Database.MediaAPI = "file:dendritejs_mediaapi.db?txns=false"
	cfg.Database.Naffka = "file:dendritejs_naffka.db?txns=false"
	cfg.Database.PublicRoomsAPI = "file:dendritejs_publicrooms.db?txns=false"
	cfg.Database.RoomServer = "file:dendritejs_roomserver.db?txns=false"
	cfg.Database.ServerKey = "file:dendritejs_serverkey.db?txns=false"
	cfg.Database.SyncAPI = "file:dendritejs_syncapi.db?txns=false"

	cfg.Matrix.ServerName = "p2p-js"
	cfg.Matrix.TrustedIDServers = []string{
		"matrix.org", "vector.im",
	}
	cfg.Matrix.KeyID = "ed25519:1337"
	cfg.Matrix.PrivateKey = generateKey()
	if err := cfg.Derive(); err != nil {
		logrus.Fatalf("Failed to derive values from config: %s", err)
	}
	base := basecomponent.NewBaseDendrite(cfg, "Monolith")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	keyDB := base.CreateKeyDB()
	federation := base.CreateFederationClient()
	keyRing := keydb.CreateKeyRing(federation.Client, keyDB)

	alias, input, query := roomserver.SetupRoomServerComponent(base)
	typingInputAPI := typingserver.SetupTypingServerComponent(base, cache.NewTypingCache())
	asQuery := appservice.SetupAppServiceAPIComponent(
		base, accountDB, deviceDB, federation, alias, query, transactions.New(),
	)
	fedSenderAPI := federationsender.SetupFederationSenderComponent(base, federation, query)

	clientapi.SetupClientAPIComponent(
		base, deviceDB, accountDB,
		federation, &keyRing, alias, input, query,
		typingInputAPI, asQuery, transactions.New(), fedSenderAPI,
	)
	federationapi.SetupFederationAPIComponent(base, accountDB, deviceDB, federation, &keyRing, alias, input, query, asQuery, fedSenderAPI)
	mediaapi.SetupMediaAPIComponent(base, deviceDB)
	publicroomsapi.SetupPublicRoomsAPIComponent(base, deviceDB, query)
	syncapi.SetupSyncAPIComponent(base, deviceDB, accountDB, query, federation, cfg)

	httpHandler := common.WrapHandlerInCORS(base.APIMux)

	http.Handle("/", httpHandler)

	// Expose the matrix APIs via libp2p-js
	if base.P2pLocalNode != nil {
		go func() {
			logrus.Info("Listening on libp2p-js host ID ", base.P2pLocalNode.Id)

			listener := go_http_js_libp2p.NewP2pListener(base.P2pLocalNode)
			defer listener.Close()
			s := &http.Server{}
			s.Serve(listener)
		}()
	}

	go func() {
		logrus.Info("Listening for service-worker fetch traffic")

		listener := go_http_js_libp2p.NewFetchListener()
		s := &http.Server{}
		go s.Serve(listener)
	}()

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}
