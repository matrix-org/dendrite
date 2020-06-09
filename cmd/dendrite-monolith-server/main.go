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

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/serverkeyapi"

	"github.com/sirupsen/logrus"
)

var (
	httpBindAddr   = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr  = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	certFile       = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile        = flag.String("tls-key", "", "The PEM private key to use for TLS")
	enableHTTPAPIs = flag.Bool("api", false, "Use HTTP APIs instead of short-circuiting (warning: exposes API endpoints!)")
)

func main() {
	cfg := basecomponent.ParseFlags(true)
	if *enableHTTPAPIs {
		// If the HTTP APIs are enabled then we need to update the Listen
		// statements in the configuration so that we know where to find
		// the API endpoints. They'll listen on the same port as the monolith
		// itself.
		addr := config.Address(*httpBindAddr)
		cfg.Listen.RoomServer = addr
		cfg.Listen.EDUServer = addr
		cfg.Listen.AppServiceAPI = addr
		cfg.Listen.FederationSender = addr
		cfg.Listen.ServerKeyAPI = addr
	}

	base := basecomponent.NewBaseDendrite(cfg, "Monolith", *enableHTTPAPIs)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	federation := base.CreateFederationClient()

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

	eduProducer := producers.NewEDUServerProducer(eduInputAPI)
	publicRoomsDB, err := storage.NewPublicRoomsServerDatabase(string(base.Cfg.Database.PublicRoomsAPI), base.Cfg.DbProperties(), cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to public rooms db")
	}

	monolith := setup.Monolith{
		Config:        base.Cfg,
		AccountDB:     accountDB,
		DeviceDB:      deviceDB,
		FedClient:     federation,
		KeyRing:       keyRing,
		KafkaConsumer: base.KafkaConsumer,
		KafkaProducer: base.KafkaProducer,

		AppserviceAPI:       asAPI,
		EDUInternalAPI:      eduInputAPI,
		EDUProducer:         eduProducer,
		FederationSenderAPI: fsAPI,
		RoomserverAPI:       rsAPI,
		ServerKeyAPI:        serverKeyAPI,

		PublicRoomsDB: publicRoomsDB,
	}
	monolith.AddAllPublicRoutes(base.PublicAPIMux)

	internal.SetupHTTPAPI(
		http.DefaultServeMux,
		base.PublicAPIMux,
		base.InternalAPIMux,
		cfg,
		base.UseHTTPAPIs,
	)

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		serv := http.Server{
			Addr:         *httpBindAddr,
			WriteTimeout: basecomponent.HTTPServerTimeout,
		}

		logrus.Info("Listening on ", serv.Addr)
		logrus.Fatal(serv.ListenAndServe())
	}()
	// Handle HTTPS if certificate and key are provided
	if *certFile != "" && *keyFile != "" {
		go func() {
			serv := http.Server{
				Addr:         *httpsBindAddr,
				WriteTimeout: basecomponent.HTTPServerTimeout,
			}

			logrus.Info("Listening on ", serv.Addr)
			logrus.Fatal(serv.ListenAndServeTLS(*certFile, *keyFile))
		}()
	}

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}
