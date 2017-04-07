package main

import (
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/clientapi/storage"
	"github.com/matrix-org/dendrite/clientapi/sync"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dugong"
	yaml "gopkg.in/yaml.v2"
)

var configPath = flag.String("config", "sync-server-config.yaml", "The path to the config file. For more information, see the config file in this repository.")
var bindAddr = flag.String("listen", ":4200", "The port to listen on.")

func setupLogging(logDir string) {
	_ = os.Mkdir(logDir, os.ModePerm)
	log.AddHook(dugong.NewFSHook(
		filepath.Join(logDir, "info.log"),
		filepath.Join(logDir, "warn.log"),
		filepath.Join(logDir, "error.log"),
		&log.TextFormatter{
			TimestampFormat:  "2006-01-02 15:04:05.000000",
			DisableColors:    true,
			DisableTimestamp: false,
			DisableSorting:   false,
		}, &dugong.DailyRotationSchedule{GZip: true},
	))
}

func loadConfig(configPath string) (*config.Sync, error) {
	contents, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg config.Sync
	if err = yaml.Unmarshal(contents, &cfg); err != nil {
		return nil, err
	}
	// check required fields
	return &cfg, nil
}

func main() {
	flag.Parse()

	if *configPath == "" {
		log.Fatal("--config must be supplied")
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Invalid config file: %s", err)
	}

	if *bindAddr == "" {
		log.Fatal("--listen must be supplied")
	}
	logDir := os.Getenv("LOG_DIR")
	if logDir != "" {
		setupLogging(logDir)
	}

	log.Info("sync server config: ", cfg)

	db, err := storage.NewSyncServerDatabase(cfg.DataSource)
	if err != nil {
		log.Panicf("startup: failed to create sync server database with data source %s : %s", cfg.DataSource, err)
	}

	rp := sync.NewRequestPool(db)

	server, err := sync.NewServer(cfg, rp, db)
	if err != nil {
		log.Panicf("startup: failed to create sync server: %s", err)
	}
	if err = server.Start(); err != nil {
		log.Panicf("startup: failed to start sync server")
	}

	log.Info("Starting sync server on ", *bindAddr)
	routing.SetupSyncServerListeners(http.DefaultServeMux, http.DefaultClient, *cfg, rp)
	log.Fatal(http.ListenAndServe(*bindAddr, nil))
}
