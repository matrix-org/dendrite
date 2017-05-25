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
	"net/http"
	"os"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/Sirupsen/logrus"
)

var (
	kafkaURIs            = strings.Split(os.Getenv("KAFKA_URIS"), ",")
	bindAddr             = os.Getenv("BIND_ADDRESS")
	logDir               = os.Getenv("LOG_DIR")
	roomserverURL        = os.Getenv("ROOMSERVER_URL")
	clientAPIOutputTopic = os.Getenv("CLIENTAPI_OUTPUT_TOPIC")
	serverName           = gomatrixserverlib.ServerName(os.Getenv("SERVER_NAME"))
	serverKey            = os.Getenv("SERVER_KEY")
	accountDataSource    = os.Getenv("ACCOUNT_DATABASE")
)

func main() {
	common.SetupLogging(logDir)
	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}
	if len(kafkaURIs) == 0 {
		// the kafka default is :9092
		kafkaURIs = []string{"localhost:9092"}
	}
	if roomserverURL == "" {
		log.Panic("No ROOMSERVER_URL environment variable found.")
	}
	if clientAPIOutputTopic == "" {
		log.Panic("No CLIENTAPI_OUTPUT_TOPIC environment variable found. This should match the roomserver input topic.")
	}
	if serverName == "" {
		serverName = "localhost"
	}

	cfg := config.ClientAPI{
		ServerName:           serverName,
		KafkaProducerURIs:    kafkaURIs,
		ClientAPIOutputTopic: clientAPIOutputTopic,
		RoomserverURL:        roomserverURL,
	}

	var err error
	cfg.KeyID, cfg.PrivateKey, err = common.ReadKey(serverKey)
	if err != nil {
		log.Panicf("Failed to load private key: %s", err)
	}

	log.Info("Starting clientapi")

	roomserverProducer, err := producers.NewRoomserverProducer(cfg.KafkaProducerURIs, cfg.ClientAPIOutputTopic)
	if err != nil {
		log.Panicf("Failed to setup kafka producers(%s): %s", cfg.KafkaProducerURIs, err)
	}

	federation := gomatrixserverlib.NewFederationClient(cfg.ServerName, cfg.KeyID, cfg.PrivateKey)

	queryAPI := api.NewRoomserverQueryAPIHTTP(cfg.RoomserverURL, nil)
	accountDB, err := accounts.NewDatabase(accountDataSource, serverName)
	if err != nil {
		log.Panicf("Failed to setup account database(%s): %s", accountDataSource, err.Error())
	}
	deviceDB, err := devices.NewDatabase(accountDataSource, serverName)
	if err != nil {
		log.Panicf("Failed to setup device database(%s): %s", accountDataSource, err.Error())
	}

	keyRing := gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			// TODO: Use perspective key fetchers for production.
			&gomatrixserverlib.DirectKeyFetcher{federation.Client},
		},
		KeyDatabase: &dummyKeyDatabase{},
	}

	routing.Setup(
		http.DefaultServeMux, http.DefaultClient, cfg, roomserverProducer,
		queryAPI, accountDB, deviceDB, federation, keyRing,
	)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}

// TODO: Implement a proper key database.
type dummyKeyDatabase struct{}

func (d *dummyKeyDatabase) FetchKeys(
	requests map[gomatrixserverlib.PublicKeyRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyRequest]gomatrixserverlib.ServerKeys, error) {
	return nil, nil
}

func (d *dummyKeyDatabase) StoreKeys(
	map[gomatrixserverlib.PublicKeyRequest]gomatrixserverlib.ServerKeys,
) error {
	return nil
}
