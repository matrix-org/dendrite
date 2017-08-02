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
	_ "net/http/pprof"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/alias"
	"github.com/matrix-org/dendrite/roomserver/input"
	"github.com/matrix-org/dendrite/roomserver/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/prometheus/client_golang/prometheus"
	sarama "gopkg.in/Shopify/sarama.v1"
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

	db, err := storage.Open(string(cfg.Database.RoomServer))
	if err != nil {
		panic(err)
	}

	kafkaProducer, err := sarama.NewSyncProducer(cfg.Kafka.Addresses, nil)
	if err != nil {
		panic(err)
	}

	inputAPI := input.RoomserverInputAPI{
		DB:                   db,
		Producer:             kafkaProducer,
		OutputRoomEventTopic: string(cfg.Kafka.Topics.OutputRoomEvent),
	}

	inputAPI.SetupHTTP(http.DefaultServeMux)

	queryAPI := query.RoomserverQueryAPI{db}

	queryAPI.SetupHTTP(http.DefaultServeMux)

	aliasAPI := alias.RoomserverAliasAPI{
		DB:       db,
		Cfg:      cfg,
		InputAPI: inputAPI,
		QueryAPI: queryAPI,
	}

	aliasAPI.SetupHTTP(http.DefaultServeMux)

	http.DefaultServeMux.Handle("/metrics", prometheus.Handler())

	log.Info("Started room server on ", cfg.Listen.RoomServer)

	// TODO: Implement clean shutdown.
	if err := http.ListenAndServe(string(cfg.Listen.RoomServer), nil); err != nil {
		panic(err)
	}
}
