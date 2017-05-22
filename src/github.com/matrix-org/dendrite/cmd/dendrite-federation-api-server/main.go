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
)

func main() {
	common.SetupLogging(logDir)
	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}

	if serverName == "" {
		serverName = "localhost"
	}

	cfg := config.FederationAPI{
		ServerName: serverName,
		// TODO: make the validity period configurable.
		ValidityPeriod: 24 * time.Hour,
	}
	cfg.TLSFingerPrints = []gomatrixserverlib.TLSFingerprint{
		{[]byte("o\xe2\xd1\x05A7g\xd6=\x10\xdfq\x9e4\xb1:/\x9co>\x01g\x1d\xb8\xbebFf]\xf0\x89N")},
	}

	var err error
	cfg.KeyID, cfg.PrivateKey, err = common.ReadKey(serverKey)
	if err != nil {
		log.Panicf("Failed to load private key: %s", err)
	}

	routing.Setup(http.DefaultServeMux, cfg)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
