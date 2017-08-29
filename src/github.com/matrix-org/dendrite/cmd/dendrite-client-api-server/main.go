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
	"github.com/matrix-org/dendrite/clientapi/consumers"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/sirupsen/logrus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

var (
	logDir     = os.Getenv("LOG_DIR")
	configPath = flag.String("config", "dendrite.yaml", "The path to the config file, For more information see the config file in this repository")
)

func main() {
	common.SetupLogging(logDir)

	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Invalid config file: %s", err)
	}

	queryAPI := api.NewRoomserverQueryAPIHTTP(cfg.RoomServerURL(), nil)
	aliasAPI := api.NewRoomserverAliasAPIHTTP(cfg.RoomServerURL(), nil)
	inputAPI := api.NewRoomserverInputAPIHTTP(cfg.RoomServerURL(), nil)

	roomserverProducer := producers.NewRoomserverProducer(inputAPI)

	kafkaProducer, err := sarama.NewSyncProducer(cfg.Kafka.Addresses, nil)
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"addresses":  cfg.Kafka.Addresses,
		}).Panic("Failed to setup kafka producers")
	}

	userUpdateProducer := &producers.UserUpdateProducer{
		Producer: kafkaProducer,
		Topic:    string(cfg.Kafka.Topics.UserUpdates),
	}

	syncProducer := &producers.SyncAPIProducer{
		Producer: kafkaProducer,
		Topic:    string(cfg.Kafka.Topics.OutputClientData),
	}

	federation := gomatrixserverlib.NewFederationClient(
		cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)

	accountDB, err := accounts.NewDatabase(string(cfg.Database.Account), cfg.Matrix.ServerName)
	if err != nil {
		log.Panicf("Failed to setup account database(%q): %s", cfg.Database.Account, err.Error())
	}
	deviceDB, err := devices.NewDatabase(string(cfg.Database.Device), cfg.Matrix.ServerName)
	if err != nil {
		log.Panicf("Failed to setup device database(%q): %s", cfg.Database.Device, err.Error())
	}
	keyDB, err := keydb.NewDatabase(string(cfg.Database.ServerKey))
	if err != nil {
		log.Panicf("Failed to setup key database(%q): %s", cfg.Database.ServerKey, err.Error())
	}

	keyRing := gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			// TODO: Use perspective key fetchers for production.
			&gomatrixserverlib.DirectKeyFetcher{Client: federation.Client},
		},
		KeyDatabase: keyDB,
	}

	kafkaConsumer, err := sarama.NewConsumer(cfg.Kafka.Addresses, nil)
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"addresses":  cfg.Kafka.Addresses,
		}).Panic("Failed to setup kafka consumers")
	}

	consumer := consumers.NewOutputRoomEvent(cfg, kafkaConsumer, accountDB, queryAPI)
	if err = consumer.Start(); err != nil {
		log.Panicf("startup: failed to start room server consumer")
	}

	log.Info("Starting client API server on ", cfg.Listen.ClientAPI)

	api := mux.NewRouter()
	routing.Setup(
		api, http.DefaultClient, *cfg, roomserverProducer,
		queryAPI, aliasAPI, accountDB, deviceDB, federation, keyRing,
		userUpdateProducer, syncProducer,
	)
	common.SetupHTTPAPI(http.DefaultServeMux, api)

	log.Fatal(http.ListenAndServe(string(cfg.Listen.ClientAPI), nil))
}
