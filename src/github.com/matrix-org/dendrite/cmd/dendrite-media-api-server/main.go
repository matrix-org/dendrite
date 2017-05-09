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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/routing"
	"github.com/matrix-org/dendrite/mediaapi/storage"

	log "github.com/Sirupsen/logrus"
)

var (
	bindAddr   = os.Getenv("BIND_ADDRESS")
	dataSource = os.Getenv("DATABASE")
	logDir     = os.Getenv("LOG_DIR")
)

func main() {
	common.SetupLogging(logDir)

	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}

	cfg := config.MediaAPI{
		ServerName:  "localhost",
		BasePath:    "/Users/robertsw/dendrite",
		MaxFileSize: 61440,
		DataSource:  dataSource,
	}

	db, err := storage.Open(cfg.DataSource)
	if err != nil {
		log.Panicln("Failed to open database:", err)
	}

	log.Info("Starting mediaapi")

	routing.Setup(http.DefaultServeMux, http.DefaultClient, cfg, db)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
