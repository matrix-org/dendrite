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
	"time"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationapi/config"
	"github.com/matrix-org/dendrite/federationapi/routing"
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
	tlsFingerprint = os.Getenv("TLS_FINGERPRINT")
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

	routing.Setup(http.DefaultServeMux, cfg)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
