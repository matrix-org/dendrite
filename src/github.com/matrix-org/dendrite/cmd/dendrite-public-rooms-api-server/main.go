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
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/publicroomsapi/routing"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"

	log "github.com/Sirupsen/logrus"
)

var configPath = flag.String("config", "dendrite.yaml", "The path to the config file. For more information, see the config file in this repository.")

func main() {
	common.SetupLogging(os.Getenv("LOG_DIR"))

	flag.Parse()

	if *configPath == "" {
		log.Fatal("--config must be supplied")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Invalid config file: %s", err)
	}

	db, err := storage.NewPublicRoomsServerDatabase(string(cfg.Database.PublicRoomsAPI))
	if err != nil {
		log.Panicf("startup: failed to create public rooms server database with data source %s : %s", cfg.Database.PublicRoomsAPI, err)
	}

	log.Info("Starting public rooms server on ", cfg.Listen.PublicRoomsAPI)

	api := mux.NewRouter()
	routing.Setup(api, db)
	common.SetupHTTPAPI(http.DefaultServeMux, api)

	log.Fatal(http.ListenAndServe(string(cfg.Listen.PublicRoomsAPI), nil))
}
