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
	"path/filepath"
	"strconv"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/routing"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/Sirupsen/logrus"
)

var (
	bindAddr   = os.Getenv("BIND_ADDRESS")
	dataSource = os.Getenv("DATABASE")
	logDir     = os.Getenv("LOG_DIR")
	serverName = os.Getenv("SERVER_NAME")
	basePath   = os.Getenv("BASE_PATH")
	// Note: if the MAX_FILE_SIZE_BYTES is set to 0, it will be unlimited
	maxFileSizeBytesString = os.Getenv("MAX_FILE_SIZE_BYTES")
)

func main() {
	common.SetupLogging(logDir)

	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}
	if basePath == "" {
		log.Panic("No BASE_PATH environment variable found.")
	}
	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		log.WithError(err).WithField("BASE_PATH", basePath).Panic("BASE_PATH is invalid (must be able to make absolute)")
	}

	if serverName == "" {
		serverName = "localhost"
	}
	maxFileSizeBytes, err := strconv.ParseInt(maxFileSizeBytesString, 10, 64)
	if err != nil {
		maxFileSizeBytes = 10 * 1024 * 1024
		log.WithError(err).WithField("MAX_FILE_SIZE_BYTES", maxFileSizeBytesString).Warnf("Failed to parse MAX_FILE_SIZE_BYTES. Defaulting to %v bytes.", maxFileSizeBytes)
	}

	cfg := &config.MediaAPI{
		ServerName:       gomatrixserverlib.ServerName(serverName),
		AbsBasePath:      types.Path(absBasePath),
		MaxFileSizeBytes: types.FileSizeBytes(maxFileSizeBytes),
		DataSource:       dataSource,
	}

	log.WithFields(log.Fields{
		"BASE_PATH":           absBasePath,
		"BIND_ADDRESS":        bindAddr,
		"DATABASE":            dataSource,
		"LOG_DIR":             logDir,
		"MAX_FILE_SIZE_BYTES": maxFileSizeBytes,
		"SERVER_NAME":         serverName,
	}).Info("Starting mediaapi")

	routing.Setup(http.DefaultServeMux, http.DefaultClient, cfg)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
