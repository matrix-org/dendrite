package main

import (
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ed25519"

	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/roomserver/api"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dugong"
	sarama "gopkg.in/Shopify/sarama.v1"
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
		KafkaProducerURIs:    []string{"localhost:9092"},
		ClientAPIOutputTopic: "roomserverInput",
		RoomserverURL:        "http://localhost:7777",
	}

	log.Info("Starting clientapi")

	producer, err := sarama.NewSyncProducer(cfg.KafkaProducerURIs, nil)
	if err != nil {
		log.Panicf("Failed to setup kafka producers(%s): %s", cfg.KafkaProducerURIs, err)
	}
	queryAPI := api.NewRoomserverQueryAPIHTTP(cfg.RoomserverURL, nil)

	routing.Setup(http.DefaultServeMux, http.DefaultClient, cfg, producer, queryAPI)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
