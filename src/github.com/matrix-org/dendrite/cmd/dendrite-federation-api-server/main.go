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
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationapi/config"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/Sirupsen/logrus"
)

var (
	bindAddr   = os.Getenv("BIND_ADDRESS")
	logDir     = os.Getenv("LOG_DIR")
	serverName = gomatrixserverlib.ServerName(os.Getenv("SERVER_NAME"))
	serverKey  = os.Getenv("SERVER_KEY")
	// Base64 encoded SHA256 TLS fingerprint of the X509 certificate used by
	// the public federation listener for this server.
	// Can be generated from a PEM certificate called "server.crt" using:
	//
	//  openssl x509 -noout -fingerprint -sha256 -inform pem -in server.crt |\
	//     python -c 'print raw_input()[19:].replace(":","").decode("hex").encode("base64").rstrip("=\n")'
	//
	tlsFingerprint       = os.Getenv("TLS_FINGERPRINT")
	kafkaURIs            = strings.Split(os.Getenv("KAFKA_URIS"), ",")
	roomserverURL        = os.Getenv("ROOMSERVER_URL")
	roomserverInputTopic = os.Getenv("TOPIC_INPUT_ROOM_EVENT")
)

func main() {
	common.SetupLogging(logDir)
	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}

	if serverName == "" {
		serverName = "localhost"
	}

	if tlsFingerprint == "" {
		log.Panic("No TLS_FINGERPRINT environment variable found.")
	}

	if len(kafkaURIs) == 0 {
		// the kafka default is :9092
		kafkaURIs = []string{"localhost:9092"}
	}

	if roomserverURL == "" {
		log.Panic("No ROOMSERVER_URL environment variable found.")
	}

	if roomserverInputTopic == "" {
		log.Panic("No TOPIC_INPUT_ROOM_EVENT environment variable found. This should match the roomserver input topic.")
	}
	cfg := config.FederationAPI{
		ServerName: serverName,
		// TODO: make the validity period configurable.
		ValidityPeriod: 24 * time.Hour,
	}

	var err error
	cfg.KeyID, cfg.PrivateKey, err = common.ReadKey(serverKey)
	if err != nil {
		log.Panicf("Failed to load private key: %s", err)
	}

	var fingerprintSHA256 []byte
	if fingerprintSHA256, err = base64.RawStdEncoding.DecodeString(tlsFingerprint); err != nil {
		log.Panicf("Failed to load TLS fingerprint: %s", err)
	}
	cfg.TLSFingerPrints = []gomatrixserverlib.TLSFingerprint{{fingerprintSHA256}}

	federation := gomatrixserverlib.NewFederationClient(cfg.ServerName, cfg.KeyID, cfg.PrivateKey)

	keyRing := gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			// TODO: Use perspective key fetchers for production.
			&gomatrixserverlib.DirectKeyFetcher{federation.Client},
		},
		KeyDatabase: &dummyKeyDatabase{},
	}
	queryAPI := api.NewRoomserverQueryAPIHTTP(roomserverURL, nil)

	roomserverProducer, err := producers.NewRoomserverProducer(kafkaURIs, roomserverInputTopic)
	if err != nil {
		log.Panicf("Failed to setup kafka producers(%s): %s", kafkaURIs, err)
	}

	routing.Setup(http.DefaultServeMux, cfg, queryAPI, roomserverProducer, keyRing)
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
