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
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/syncapi/consumers"
	"github.com/matrix-org/dendrite/syncapi/routing"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"

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

	log.Info("config: ", cfg)

	db, err := storage.NewSyncServerDatabase(string(cfg.Database.SyncAPI))
	if err != nil {
		log.Panicf("startup: failed to create sync server database with data source %s : %s", cfg.Database.SyncAPI, err)
	}

	deviceDB, err := devices.NewDatabase(string(cfg.Database.Device), cfg.Matrix.ServerName)
	if err != nil {
		log.Panicf("startup: failed to create device database with data source %s : %s", cfg.Database.Device, err)
	}

	adb, err := accounts.NewDatabase(string(cfg.Database.Account), cfg.Matrix.ServerName)
	if err != nil {
		log.Panicf("startup: failed to create account database with data source %s : %s", cfg.Database.Account, err)
	}

	pos, err := db.SyncStreamPosition()
	if err != nil {
		log.Panicf("startup: failed to get latest sync stream position : %s", err)
	}

	n := sync.NewNotifier(types.StreamPosition(pos))
	if err = n.Load(db); err != nil {
		log.Panicf("startup: failed to set up notifier: %s", err)
	}
	roomConsumer, err := consumers.NewOutputRoomEvent(cfg, n, db)
	if err != nil {
		log.Panicf("startup: failed to create room server consumer: %s", err)
	}
	if err = roomConsumer.Start(); err != nil {
		log.Panicf("startup: failed to start room server consumer: %s", err)
	}
	clientConsumer, err := consumers.NewOutputClientData(cfg, n, db)
	if err != nil {
		log.Panicf("startup: failed to create client API server consumer: %s", err)
	}
	if err = clientConsumer.Start(); err != nil {
		log.Panicf("startup: failed to start client API server consumer: %s", err)
	}

	log.Info("Starting sync server on ", cfg.Listen.SyncAPI)

	api := mux.NewRouter()
	routing.Setup(api, sync.NewRequestPool(db, n, adb), deviceDB)
	common.SetupHTTPAPI(http.DefaultServeMux, api)

	log.Fatal(http.ListenAndServe(string(cfg.Listen.SyncAPI), nil))
}
