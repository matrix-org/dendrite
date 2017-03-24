package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/clientapi/storage"
	"github.com/matrix-org/dendrite/clientapi/sync"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dugong"
)

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

func main() {
	bindAddr := os.Getenv("BIND_ADDRESS")
	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}
	logDir := os.Getenv("LOG_DIR")
	if logDir != "" {
		setupLogging(logDir)
	}

	cfg := config.Sync{
		KafkaConsumerURIs:     []string{"localhost:9092"},
		RoomserverOutputTopic: "roomserverOutput",
		DataSource:            "postgres://user:pass@localhost/dendrite-sync-server?sslmode=disable",
	}

	log.Info("Starting sync server")

	db, err := storage.NewSyncServerDatabase(cfg.DataSource)
	if err != nil {
		log.Panicf("startup: failed to create sync server database with data source %s : %s", cfg.DataSource, err)
	}

	server, err := sync.NewServer(&cfg, db)
	if err != nil {
		log.Panicf("startup: failed to create sync server: %s", err)
	}
	if err = server.Start(); err != nil {
		log.Panicf("startup: failed to start sync server")
	}

	routing.SetupSyncServerListeners(http.DefaultServeMux, http.DefaultClient, cfg)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
