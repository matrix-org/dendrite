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
	"fmt"
	"os"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/currentstateserver"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/serverkeyapi"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	httpBindAddr   = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr  = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	certFile       = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile        = flag.String("tls-key", "", "The PEM private key to use for TLS")
	enableHTTPAPIs = flag.Bool("api", false, "Use HTTP APIs instead of short-circuiting (warning: exposes API endpoints!)")
	traceInternal  = os.Getenv("DENDRITE_TRACE_INTERNAL") == "1"
)

func main() {
	cfg := setup.ParseFlags(true)
	httpAddr := config.HTTPAddress("http://" + *httpBindAddr)
	httpsAddr := config.HTTPAddress("https://" + *httpsBindAddr)

	if *enableHTTPAPIs {
		// If the HTTP APIs are enabled then we need to update the Listen
		// statements in the configuration so that we know where to find
		// the API endpoints. They'll listen on the same port as the monolith
		// itself.
		cfg.AppServiceAPI.InternalAPI.Connect = httpAddr
		cfg.ClientAPI.InternalAPI.Connect = httpAddr
		cfg.CurrentStateServer.InternalAPI.Connect = httpAddr
		cfg.EDUServer.InternalAPI.Connect = httpAddr
		cfg.FederationAPI.InternalAPI.Connect = httpAddr
		cfg.FederationSender.InternalAPI.Connect = httpAddr
		cfg.KeyServer.InternalAPI.Connect = httpAddr
		cfg.MediaAPI.InternalAPI.Connect = httpAddr
		cfg.RoomServer.InternalAPI.Connect = httpAddr
		cfg.ServerKeyAPI.InternalAPI.Connect = httpAddr
		cfg.SyncAPI.InternalAPI.Connect = httpAddr
	}

	base := setup.NewBaseDendrite(cfg, "Monolith", *enableHTTPAPIs)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	federation := base.CreateFederationClient()

	serverKeyAPI := serverkeyapi.NewInternalAPI(
		&base.Cfg.ServerKeyAPI, federation, base.Caches,
	)
	if base.UseHTTPAPIs {
		serverkeyapi.AddInternalRoutes(base.InternalAPIMux, serverKeyAPI, base.Caches)
		serverKeyAPI = base.ServerKeyAPIClient()
	}
	keyRing := serverKeyAPI.KeyRing()
	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, federation, base.KafkaProducer)
	userAPI := userapi.NewInternalAPI(accountDB, deviceDB, cfg.Global.ServerName, cfg.Derived.ApplicationServices, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	rsImpl := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)
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

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), userAPI,
	)
	if base.UseHTTPAPIs {
		eduserver.AddInternalRoutes(base.InternalAPIMux, eduInputAPI)
		eduInputAPI = base.EDUServerClient()
	}

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
	if base.UseHTTPAPIs {
		appservice.AddInternalRoutes(base.InternalAPIMux, asAPI)
		asAPI = base.AppserviceHTTPClient()
	}

	stateAPI := currentstateserver.NewInternalAPI(&base.Cfg.CurrentStateServer, base.KafkaConsumer)

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, stateAPI, keyRing,
	)
	if base.UseHTTPAPIs {
		federationsender.AddInternalRoutes(base.InternalAPIMux, fsAPI)
		fsAPI = base.FederationSenderHTTPClient()
	}
	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsImpl.SetFederationSenderAPI(fsAPI)

	monolith := setup.Monolith{
		Config:        base.Cfg,
		AccountDB:     accountDB,
		DeviceDB:      deviceDB,
		Client:        gomatrixserverlib.NewClient(cfg.FederationSender.DisableTLSValidation),
		FedClient:     federation,
		KeyRing:       keyRing,
		KafkaConsumer: base.KafkaConsumer,
		KafkaProducer: base.KafkaProducer,

		AppserviceAPI:       asAPI,
		EDUInternalAPI:      eduInputAPI,
		FederationSenderAPI: fsAPI,
		RoomserverAPI:       rsAPI,
		ServerKeyAPI:        serverKeyAPI,
		StateAPI:            stateAPI,
		UserAPI:             userAPI,
		KeyAPI:              keyAPI,
	}
	monolith.AddAllPublicRoutes(base.PublicAPIMux)

	fmt.Printf("Public: %+v\n", base.PublicAPIMux)
	fmt.Printf("Internal: %+v\n", base.InternalAPIMux)

	/*
		httputil.SetupHTTPAPI(
			base.BaseMux,
			base.PublicAPIMux,
			base.InternalAPIMux,
			&cfg.Global,
			base.UseHTTPAPIs,
		)
	*/

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		base.SetupAndServeHTTP(
			config.HTTPAddress(httpAddr), // internal API
			config.HTTPAddress(httpAddr), // external API
		)
	}()
	// Handle HTTPS if certificate and key are provided
	_ = httpsAddr
	/*
		if *certFile != "" && *keyFile != "" {
			go func() {
				serv := http.Server{
					Addr:         config.HTTPAddress(httpsAddr).,
					WriteTimeout: setup.HTTPServerTimeout,
					Handler:      base.BaseMux,
				}

				logrus.Info("Listening on ", serv.Addr)
				logrus.Fatal(serv.ListenAndServeTLS(*certFile, *keyFile))
			}()
		}
	*/

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}
