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
	"io/ioutil"
	"net/http"
	"os"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/config"
	"github.com/matrix-org/dendrite/syncapi/consumers"
	"github.com/matrix-org/dendrite/syncapi/routing"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"

	log "github.com/Sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

var configPath = flag.String("config", "sync-server-config.yaml", "The path to the config file. For more information, see the config file in this repository.")
var bindAddr = flag.String("listen", ":4200", "The port to listen on.")

func loadConfig(configPath string) (*config.Sync, error) {
	contents, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg config.Sync
	if err = yaml.Unmarshal(contents, &cfg); err != nil {
		return nil, err
	}
	// check required fields
	if cfg.ServerName == "" {
		log.Fatalf("'server_name' must be supplied in %s", configPath)
	}
	return &cfg, nil
}

func main() {
	common.SetupLogging(os.Getenv("LOG_DIR"))

	flag.Parse()

	if *configPath == "" {
		log.Fatal("--config must be supplied")
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Invalid config file: %s", err)
	}

	if *bindAddr == "" {
		log.Fatal("--listen must be supplied")
	}

	log.Info("sync server config: ", cfg)

	db, err := storage.NewSyncServerDatabase(cfg.DataSource)
	if err != nil {
		log.Panicf("startup: failed to create sync server database with data source %s : %s", cfg.DataSource, err)
	}

	deviceDB, err := devices.NewDatabase(cfg.AccountDataSource, cfg.ServerName)
	if err != nil {
		log.Panicf("startup: failed to create device database with data source %s : %s", cfg.DataSource, err)
	}

	pos, err := db.SyncStreamPosition()
	if err != nil {
		log.Panicf("startup: failed to get latest sync stream position : %s", err)
	}

	n := sync.NewNotifier(types.StreamPosition(pos))
	if err := n.Load(db); err != nil {
		log.Panicf("startup: failed to set up notifier: %s", err)
	}
	consumer, err := consumers.NewOutputRoomEvent(cfg, n, db)
	if err != nil {
		log.Panicf("startup: failed to create room server consumer: %s", err)
	}
	if err = consumer.Start(); err != nil {
		log.Panicf("startup: failed to start room server consumer")
	}

	log.Info("Starting sync server on ", *bindAddr)
	routing.SetupSyncServerListeners(http.DefaultServeMux, http.DefaultClient, *cfg, sync.NewRequestPool(db, n), deviceDB)
	log.Fatal(http.ListenAndServe(*bindAddr, nil))
}
