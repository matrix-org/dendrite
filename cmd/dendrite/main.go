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

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	basepkg "github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs"
	"github.com/matrix-org/dendrite/userapi"
)

var (
	unixSocket = flag.String("unix-socket", "",
		"EXPERIMENTAL(unstable): The HTTP listening unix socket for the server (disables http[s]-bind-address feature)",
	)
	unixSocketPermission = flag.String("unix-socket-permission", "755",
		"EXPERIMENTAL(unstable): The HTTP listening unix socket permission for the server (in chmod format like 755)",
	)
	httpBindAddr  = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	certFile      = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile       = flag.String("tls-key", "", "The PEM private key to use for TLS")
)

func main() {
	cfg := setup.ParseFlags(true)
	httpAddr := config.ServerAddress{}
	httpsAddr := config.ServerAddress{}
	if *unixSocket == "" {
		http, err := config.HTTPAddress("http://" + *httpBindAddr)
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to parse http address")
		}
		httpAddr = http
		https, err := config.HTTPAddress("https://" + *httpsBindAddr)
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to parse https address")
		}
		httpsAddr = https
	} else {
		socket, err := config.UnixSocketAddress(*unixSocket, *unixSocketPermission)
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to parse unix socket")
		}
		httpAddr = socket
	}

	options := []basepkg.BaseDendriteOptions{}

	base := basepkg.NewBaseDendrite(cfg, options...)
	defer base.Close() // nolint: errcheck

	federation := base.CreateFederationClient()

	caches := caching.NewRistrettoCache(base.Cfg.Global.Cache.EstimatedMaxSize, base.Cfg.Global.Cache.MaxAge, caching.EnableMetrics)
	natsInstance := jetstream.NATSInstance{}
	rsAPI := roomserver.NewInternalAPI(base, &natsInstance, caches)
	fsAPI := federationapi.NewInternalAPI(
		base.ProcessContext, base.Cfg, base.ConnectionManager, &natsInstance, federation, rsAPI, caches, nil, false,
	)

	keyRing := fsAPI.KeyRing()

	userAPI := userapi.NewInternalAPI(base, &natsInstance, rsAPI, federation)

	asAPI := appservice.NewInternalAPI(base.ProcessContext, base.Cfg, &natsInstance, userAPI, rsAPI)

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this
	// dependency. Other components also need updating after their dependencies are up.
	rsAPI.SetFederationAPI(fsAPI, keyRing)
	rsAPI.SetAppserviceAPI(asAPI)
	rsAPI.SetUserAPI(userAPI)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		Client:    base.CreateClient(),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI: asAPI,
		// always use the concrete impl here even in -http mode because adding public routes
		// must be done on the concrete impl not an HTTP client else fedapi will call itself
		FederationAPI: fsAPI,
		RoomserverAPI: rsAPI,
		UserAPI:       userAPI,
	}
	monolith.AddAllPublicRoutes(base, &natsInstance, caches)

	if len(base.Cfg.MSCs.MSCs) > 0 {
		if err := mscs.Enable(base, &monolith, caches); err != nil {
			logrus.WithError(err).Fatalf("Failed to enable MSCs")
		}
	}

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		base.SetupAndServeHTTP(httpAddr, nil, nil)
	}()
	// Handle HTTPS if certificate and key are provided
	if *unixSocket == "" && *certFile != "" && *keyFile != "" {
		go func() {
			base.SetupAndServeHTTP(httpsAddr, certFile, keyFile)
		}()
	}

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	base.WaitForShutdown()
}
