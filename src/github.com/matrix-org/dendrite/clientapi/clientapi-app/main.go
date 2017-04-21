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
	"net/url"
	"strings"

	"golang.org/x/crypto/ed25519"

	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/roomserver/api"

	log "github.com/Sirupsen/logrus"
)

// App is a function that configures and starts a client API server
func App(host string, kafkaURIsStr string, roomserverURL string, topicPrefix string) {
	if host == "" {
		log.Panic("No host specified.")
	}
	hostURL, err := url.Parse(host)
	if hostURL.Port() == "" {
		host += ":7778"
	}
	kafkaURIs := strings.Split(kafkaURIsStr, ",")
	if len(kafkaURIs) == 0 {
		// the kafka default is :9092
		kafkaURIs = []string{"localhost:9092"}
	}
	if roomserverURL == "" {
		log.Panic("No roomserver host specified.")
	}

	clientAPIOutputTopic := fmt.Sprintf("%sroomserver_input_topic", topicPrefix)

	// TODO: Rather than generating a new key on every startup, we should be
	//       reading a PEM formatted file instead.
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Panicf("Failed to generate private key: %s", err)
	}

	cfg := config.ClientAPI{
		ServerName:           "localhost",
		KeyID:                "ed25519:something",
		PrivateKey:           privKey,
		KafkaProducerURIs:    kafkaURIs,
		ClientAPIOutputTopic: clientAPIOutputTopic,
		RoomserverURL:        roomserverURL,
	}

	log.Infoln("clientapi host:", host)
	log.Infoln("clientapi output topic:", clientAPIOutputTopic)
	log.Infoln("kafka URIs:", kafkaURIs)
	log.Infoln("roomserver URL:", roomserverURL)
	log.Info("Starting clientapi")

	roomserverProducer, err := producers.NewRoomserverProducer(cfg.KafkaProducerURIs, cfg.ClientAPIOutputTopic)
	if err != nil {
		log.Panicf("Failed to setup kafka producers(%s): %s", cfg.KafkaProducerURIs, err)
	}

	queryAPI := api.NewRoomserverQueryAPIHTTP(cfg.RoomserverURL, nil)

	routing.Setup(http.DefaultServeMux, http.DefaultClient, cfg, roomserverProducer, queryAPI)
	log.Fatal(http.ListenAndServe(host, nil))
}
