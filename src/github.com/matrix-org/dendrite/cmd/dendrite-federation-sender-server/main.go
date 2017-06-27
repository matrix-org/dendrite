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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/federationsender/consumers"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"

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

	db, err := storage.NewDatabase(string(cfg.Database.FederationSender))
	if err != nil {
		log.Panicf("startup: failed to create federation sender database with data source %s : %s", cfg.Database.FederationSender, err)
	}

	federation := gomatrixserverlib.NewFederationClient(
		cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)

	queues := queue.NewOutgoingQueues(cfg.Matrix.ServerName, federation)

	consumer, err := consumers.NewOutputRoomEvent(cfg, queues, db)
	if err != nil {
		log.WithError(err).Panicf("startup: failed to create room server consumer")
	}
	if err = consumer.Start(); err != nil {
		log.WithError(err).Panicf("startup: failed to start room server consumer")
	}

	http.DefaultServeMux.Handle("/metrics", prometheus.Handler())

	if err := http.ListenAndServe(string(cfg.Listen.FederationSender), nil); err != nil {
		panic(err)
	}
}
