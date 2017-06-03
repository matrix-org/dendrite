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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/routing"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"

	log "github.com/Sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

var (
	bindAddr   = os.Getenv("BIND_ADDRESS")
	dataSource = os.Getenv("DATABASE")
	logDir     = os.Getenv("LOG_DIR")
	serverName = os.Getenv("SERVER_NAME")
	basePath   = os.Getenv("BASE_PATH")
	// Note: if the MAX_FILE_SIZE_BYTES is set to 0, it will be unlimited
	maxFileSizeBytesString = os.Getenv("MAX_FILE_SIZE_BYTES")
	configPath             = os.Getenv("CONFIG_PATH")
)

func main() {
	common.SetupLogging(logDir)

	log.WithFields(log.Fields{
		"BIND_ADDRESS":        bindAddr,
		"DATABASE":            dataSource,
		"LOG_DIR":             logDir,
		"SERVER_NAME":         serverName,
		"BASE_PATH":           basePath,
		"MAX_FILE_SIZE_BYTES": maxFileSizeBytesString,
		"CONFIG_PATH":         configPath,
	}).Info("Loading configuration based on config file and environment variables")

	cfg, err := configureServer()
	if err != nil {
		log.WithError(err).Fatal("Invalid configuration")
	}

	db, err := storage.Open(cfg.DataSource)
	if err != nil {
		log.WithError(err).Panic("Failed to open database")
	}

	log.WithFields(log.Fields{
		"BIND_ADDRESS":      bindAddr,
		"LOG_DIR":           logDir,
		"CONFIG_PATH":       configPath,
		"ServerName":        cfg.ServerName,
		"AbsBasePath":       cfg.AbsBasePath,
		"MaxFileSizeBytes":  *cfg.MaxFileSizeBytes,
		"DataSource":        cfg.DataSource,
		"DynamicThumbnails": cfg.DynamicThumbnails,
		"ThumbnailSizes":    cfg.ThumbnailSizes,
	}).Info("Starting mediaapi server with configuration")

	routing.Setup(http.DefaultServeMux, http.DefaultClient, cfg, db)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}

// configureServer loads configuration from a yaml file and overrides with environment variables
func configureServer() (*config.MediaAPI, error) {
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.WithError(err).Fatal("Invalid config file")
	}

	// override values from environment variables
	applyOverrides(cfg)

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// FIXME: make common somehow? copied from sync api
func loadConfig(configPath string) (*config.MediaAPI, error) {
	contents, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg config.MediaAPI
	if err = yaml.Unmarshal(contents, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyOverrides(cfg *config.MediaAPI) {
	if serverName != "" {
		if cfg.ServerName != "" {
			log.WithFields(log.Fields{
				"server_name": cfg.ServerName,
				"SERVER_NAME": serverName,
			}).Info("Overriding server_name from config file with environment variable")
		}
		cfg.ServerName = gomatrixserverlib.ServerName(serverName)
	}
	if cfg.ServerName == "" {
		log.Info("ServerName not set. Defaulting to 'localhost'.")
		cfg.ServerName = "localhost"
	}

	if basePath != "" {
		if cfg.BasePath != "" {
			log.WithFields(log.Fields{
				"base_path": cfg.BasePath,
				"BASE_PATH": basePath,
			}).Info("Overriding base_path from config file with environment variable")
		}
		cfg.BasePath = types.Path(basePath)
	}

	if maxFileSizeBytesString != "" {
		if cfg.MaxFileSizeBytes != nil {
			log.WithFields(log.Fields{
				"max_file_size_bytes": *cfg.MaxFileSizeBytes,
				"MAX_FILE_SIZE_BYTES": maxFileSizeBytesString,
			}).Info("Overriding max_file_size_bytes from config file with environment variable")
		}
		maxFileSizeBytesInt, err := strconv.ParseInt(maxFileSizeBytesString, 10, 64)
		if err != nil {
			maxFileSizeBytesInt = 10 * 1024 * 1024
			log.WithError(err).WithField(
				"MAX_FILE_SIZE_BYTES", maxFileSizeBytesString,
			).Infof("MAX_FILE_SIZE_BYTES not set? Defaulting to %v bytes.", maxFileSizeBytesInt)
		}
		maxFileSizeBytes := types.FileSizeBytes(maxFileSizeBytesInt)
		cfg.MaxFileSizeBytes = &maxFileSizeBytes
	}

	if dataSource != "" {
		if cfg.DataSource != "" {
			log.WithFields(log.Fields{
				"database": cfg.DataSource,
				"DATABASE": dataSource,
			}).Info("Overriding database from config file with environment variable")
		}
		cfg.DataSource = dataSource
	}
}

func validateConfig(cfg *config.MediaAPI) error {
	if bindAddr == "" {
		return fmt.Errorf("no BIND_ADDRESS environment variable found")
	}

	absBasePath, err := getAbsolutePath(cfg.BasePath)
	if err != nil {
		return fmt.Errorf("invalid base path (%v): %q", cfg.BasePath, err)
	}
	cfg.AbsBasePath = types.Path(absBasePath)

	if *cfg.MaxFileSizeBytes < 0 {
		return fmt.Errorf("invalid max file size bytes (%v)", *cfg.MaxFileSizeBytes)
	}

	if cfg.DataSource == "" {
		return fmt.Errorf("invalid database (%v)", cfg.DataSource)
	}

	for _, config := range cfg.ThumbnailSizes {
		if config.Width <= 0 || config.Height <= 0 {
			return fmt.Errorf("invalid thumbnail size %vx%v", config.Width, config.Height)
		}
	}

	return nil
}

func getAbsolutePath(basePath types.Path) (types.Path, error) {
	var err error
	if basePath == "" {
		var wd string
		wd, err = os.Getwd()
		return types.Path(wd), err
	}
	// Note: If we got here len(basePath) >= 1
	if basePath[0] == '~' {
		basePath, err = expandHomeDir(basePath)
		if err != nil {
			return "", err
		}
	}
	absBasePath, err := filepath.Abs(string(basePath))
	return types.Path(absBasePath), err
}

// expandHomeDir parses paths beginning with ~/path or ~user/path and replaces the home directory part
func expandHomeDir(basePath types.Path) (types.Path, error) {
	slash := strings.Index(string(basePath), "/")
	if slash == -1 {
		// pretend the slash is after the path as none was found within the string
		// simplifies code using slash below
		slash = len(basePath)
	}
	var usr *user.User
	var err error
	if slash == 1 {
		// basePath is ~ or ~/path
		usr, err = user.Current()
		if err != nil {
			return "", fmt.Errorf("failed to get user's home directory: %q", err)
		}
	} else {
		// slash > 1
		// basePath is ~user or ~user/path
		usr, err = user.Lookup(string(basePath[1:slash]))
		if err != nil {
			return "", fmt.Errorf("failed to get user's home directory: %q", err)
		}
	}
	return types.Path(filepath.Join(usr.HomeDir, string(basePath[slash:]))), nil
}
