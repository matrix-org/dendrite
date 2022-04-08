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
	"os"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup"
	basepkg "github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs"
	"github.com/matrix-org/dendrite/userapi"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
)

var (
	httpBindAddr   = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr  = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	apiBindAddr    = flag.String("api-bind-address", "localhost:18008", "The HTTP listening port for the internal HTTP APIs (if -api is enabled)")
	certFile       = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile        = flag.String("tls-key", "", "The PEM private key to use for TLS")
	enableHTTPAPIs = flag.Bool("api", false, "Use HTTP APIs instead of short-circuiting (warning: exposes API endpoints!)")
	traceInternal  = os.Getenv("DENDRITE_TRACE_INTERNAL") == "1"
)

func main() {
	cfg := setup.ParseFlags(true)
	httpAddr := config.HTTPAddress("http://" + *httpBindAddr)
	httpsAddr := config.HTTPAddress("https://" + *httpsBindAddr)
	httpAPIAddr := httpAddr
	options := []basepkg.BaseDendriteOptions{}
	if *enableHTTPAPIs {
		logrus.Warnf("DANGER! The -api option is enabled, exposing internal APIs on %q!", *apiBindAddr)
		httpAPIAddr = config.HTTPAddress("http://" + *apiBindAddr)
		// If the HTTP APIs are enabled then we need to update the Listen
		// statements in the configuration so that we know where to find
		// the API endpoints. They'll listen on the same port as the monolith
		// itself.
		cfg.AppServiceAPI.InternalAPI.Connect = httpAPIAddr
		cfg.ClientAPI.InternalAPI.Connect = httpAPIAddr
		cfg.FederationAPI.InternalAPI.Connect = httpAPIAddr
		cfg.KeyServer.InternalAPI.Connect = httpAPIAddr
		cfg.MediaAPI.InternalAPI.Connect = httpAPIAddr
		cfg.RoomServer.InternalAPI.Connect = httpAPIAddr
		cfg.SyncAPI.InternalAPI.Connect = httpAPIAddr
		cfg.UserAPI.InternalAPI.Connect = httpAPIAddr
		options = append(options, basepkg.UseHTTPAPIs)
	}

	base := basepkg.NewBaseDendrite(cfg, "Monolith", options...)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := base.CreateFederationClient()

	rsImpl := roomserver.NewInternalAPI(base)
	// call functions directly on the impl unless running in HTTP mode
	rsAPI := rsImpl
	if base.UseHTTPAPIs {
		roomserver.AddInternalRoutes(base.InternalAPIMux, rsImpl)
		rsAPI = base.RoomserverHTTPClient()
	}
	if traceInternal {
		rsAPI = &api.RoomserverInternalAPITrace{
			Impl: rsAPI,
		}
	}

	fsAPI := federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, nil, false,
	)
	if base.UseHTTPAPIs {
		federationapi.AddInternalRoutes(base.InternalAPIMux, fsAPI)
		fsAPI = base.FederationAPIHTTPClient()
	}
	keyRing := fsAPI.KeyRing()

	keyImpl := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, fsAPI)
	keyAPI := keyImpl
	if base.UseHTTPAPIs {
		keyserver.AddInternalRoutes(base.InternalAPIMux, keyAPI)
		keyAPI = base.KeyServerHTTPClient()
	}

	pgClient := base.PushGatewayHTTPClient()
	userImpl := userapi.NewInternalAPI(base, accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI, rsAPI, pgClient)
	userAPI := userImpl
	if base.UseHTTPAPIs {
		userapi.AddInternalRoutes(base.InternalAPIMux, userAPI)
		userAPI = base.UserAPIClient()
	}
	if traceInternal {
		userAPI = &uapi.UserInternalAPITrace{
			Impl: userAPI,
		}
	}

	// TODO: This should use userAPI, not userImpl, but the appservice setup races with
	// the listeners and panics at startup if it tries to create appservice accounts
	// before the listeners are up.
	asAPI := appservice.NewInternalAPI(base, userImpl, rsAPI)
	if base.UseHTTPAPIs {
		appservice.AddInternalRoutes(base.InternalAPIMux, asAPI)
		asAPI = base.AppserviceHTTPClient()
	}

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this
	// dependency. Other components also need updating after their dependencies are up.
	rsImpl.SetFederationAPI(fsAPI, keyRing)
	rsImpl.SetAppserviceAPI(asAPI)
	rsImpl.SetUserAPI(userAPI)
	keyImpl.SetUserAPI(userAPI)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    base.CreateClient(),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI: asAPI, FederationAPI: fsAPI,
		RoomserverAPI: rsAPI,
		UserAPI:       userAPI,
		KeyAPI:        keyAPI,
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

	if len(base.Cfg.MSCs.MSCs) > 0 {
		if err := mscs.Enable(base, &monolith); err != nil {
			logrus.WithError(err).Fatalf("Failed to enable MSCs")
		}
	}

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		base.SetupAndServeHTTP(
			httpAPIAddr, // internal API
			httpAddr,    // external API
			nil, nil,    // TLS settings
		)
	}()
	// Handle HTTPS if certificate and key are provided
	if *certFile != "" && *keyFile != "" {
		go func() {
			base.SetupAndServeHTTP(
				basepkg.NoListener, // internal API
				httpsAddr,          // external API
				certFile, keyFile,  // TLS settings
			)
		}()
	}

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	base.WaitForShutdown()
}
