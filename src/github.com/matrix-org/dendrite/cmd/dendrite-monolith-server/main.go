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
	"flag"
	"net/http"

	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/common/transactions"

	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/mediaapi"
	"github.com/matrix-org/dendrite/publicroomsapi"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/syncapi"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	httpBindAddr  = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	certFile      = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile       = flag.String("tls-key", "", "The PEM private key to use for TLS")
)

func main() {
	cfg := basecomponent.ParseMonolithFlags()
	base := basecomponent.NewBaseDendrite(cfg, "Monolith")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	keyDB := base.CreateKeyDB()
	federation := base.CreateFederationClient()
	keyRing := keydb.CreateKeyRing(federation.Client, keyDB)

	alias, input, query := roomserver.SetupRoomServerComponent(base)

	clientapi.SetupClientAPIComponent(
		base, deviceDB, accountDB,
		federation, &keyRing, alias, input, query,
		transactions.New(),
	)
	federationapi.SetupFederationAPIComponent(base, accountDB, deviceDB, federation, &keyRing, alias, input, query)
	federationsender.SetupFederationSenderComponent(base, federation, query)
	mediaapi.SetupMediaAPIComponent(base, deviceDB)
	publicroomsapi.SetupPublicRoomsAPIComponent(base, deviceDB)
	syncapi.SetupSyncAPIComponent(base, deviceDB, accountDB, query)

	httpHandler := common.WrapHandlerInCORS(base.APIMux)

	// Set up the API endpoints we handle. /metrics is for prometheus, and is
	// not wrapped by CORS, while everything else is
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/", httpHandler)

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		logrus.Info("Listening on ", *httpBindAddr)
		logrus.Fatal(http.ListenAndServe(*httpBindAddr, nil))
	}()
	// Handle HTTPS if certificate and key are provided
	go func() {
		if *certFile != "" && *keyFile != "" {
			logrus.Info("Listening on ", *httpsBindAddr)
			logrus.Fatal(http.ListenAndServeTLS(*httpsBindAddr, *certFile, *keyFile, nil))
		}
	}()

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}
