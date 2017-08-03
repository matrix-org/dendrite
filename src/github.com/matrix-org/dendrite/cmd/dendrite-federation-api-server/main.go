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
	"os"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/Sirupsen/logrus"
)

var (
	logDir     = os.Getenv("LOG_DIR")
	configPath = flag.String("config", "dendrite.yaml", "The path to the config file. For more information, see the config file in this repository.")
)

func main() {
	common.SetupLogging(logDir)

	flag.Parse()

	if *configPath == "" {
		log.Fatal("--config must be supplied")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Invalid config file: %s", err)
	}

	federation := gomatrixserverlib.NewFederationClient(
		cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)

	keyDB, err := keydb.NewDatabase(string(cfg.Database.ServerKey))
	if err != nil {
		log.Panicf("Failed to setup key database(%q): %s", cfg.Database.ServerKey, err.Error())
	}

	keyRing := gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			// TODO: Use perspective key fetchers for production.
			&gomatrixserverlib.DirectKeyFetcher{federation.Client},
		},
		KeyDatabase: keyDB,
	}

	queryAPI := api.NewRoomserverQueryAPIHTTP(cfg.RoomServerURL(), nil)

	roomserverProducer := producers.NewRoomserverProducer(cfg.RoomServerURL())

	if err != nil {
		log.Panicf("Failed to setup kafka producers(%s): %s", cfg.Kafka.Addresses, err)
	}

	log.Info("Starting federation API server on ", cfg.Listen.FederationAPI)

	api := mux.NewRouter()
	routing.Setup(api, *cfg, queryAPI, roomserverProducer, keyRing, federation)
	common.SetupHTTPAPI(http.DefaultServeMux, api)

	log.Fatal(http.ListenAndServe(string(cfg.Listen.FederationAPI), nil))
}
