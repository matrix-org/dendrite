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

package clientapi

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/ed25519"

	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/roomserver/api"

	log "github.com/Sirupsen/logrus"
)

// App is a function that configures and starts a client API server
func App(address string, kafkaAddressesStr string, roomserverURL string, topicPrefix string) {
	if address == "" {
		log.Panic("No address specified.")
	}
	kafkaAddresses := strings.Split(kafkaAddressesStr, ",")
	if len(kafkaAddresses) == 0 {
		// the kafka default is :9092
		kafkaAddresses = []string{"localhost:9092"}
	}
	if roomserverURL == "" {
		log.Panic("No roomserver URL specified.")
	}

	clientAPIOutputTopic := fmt.Sprintf("%sroomserver_input_topic", topicPrefix)

	// TODO: Rather than generating a new key on every startup, we should be
	//       reading a PEM formatted file instead.
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Panicf("Failed to generate private key: %s", err)
	}

	cfg := config.ClientAPI{
		ServerName:             "localhost",
		KeyID:                  "ed25519:something",
		PrivateKey:             privKey,
		KafkaProducerAddresses: kafkaAddresses,
		ClientAPIOutputTopic:   clientAPIOutputTopic,
		RoomserverURL:          roomserverURL,
	}

	log.Infoln("clientapi address:", address)
	log.Infoln("clientapi output topic:", clientAPIOutputTopic)
	log.Infoln("kafka addresses:", kafkaAddresses)
	log.Infoln("roomserver URL:", roomserverURL)
	log.Info("Starting clientapi")

	roomserverProducer, err := producers.NewRoomserverProducer(cfg.KafkaProducerAddresses, cfg.ClientAPIOutputTopic)
	if err != nil {
		log.Panicf("Failed to setup kafka producers(%s): %s", cfg.KafkaProducerAddresses, err)
	}

	queryAPI := api.NewRoomserverQueryAPIHTTP(cfg.RoomserverURL, nil)

	routing.Setup(http.DefaultServeMux, http.DefaultClient, cfg, roomserverProducer, queryAPI)
	log.Fatal(http.ListenAndServe(address, nil))
}
