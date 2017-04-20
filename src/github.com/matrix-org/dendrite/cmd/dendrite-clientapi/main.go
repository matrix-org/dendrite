package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ed25519"

	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/roomserver/api"

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

var (
	kafkaURIs            = strings.Split(os.Getenv("KAFKA_URIS"), ",")
	bindAddr             = os.Getenv("BIND_ADDRESS")
	logDir               = os.Getenv("LOG_DIR")
	roomserverURL        = os.Getenv("ROOMSERVER_URL")
	clientAPIOutputTopic = os.Getenv("CLIENTAPI_OUTPUT_TOPIC")
)

func main() {
	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}
	if logDir != "" {
		setupLogging(logDir)
	}
	if len(kafkaURIs) == 0 {
		// the kafka default is :9092
		kafkaURIs = []string{"localhost:9092"}
	}
	if roomserverURL == "" {
		log.Panic("No ROOMSERVER_URL environment variable found.")
	}
	if clientAPIOutputTopic == "" {
		log.Panic("No CLIENTAPI_OUTPUT_TOPIC environment variable found. This should match the roomserver input topic.")
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
		KafkaProducerURIs:    kafkaURIs,
		ClientAPIOutputTopic: clientAPIOutputTopic,
		RoomserverURL:        roomserverURL,
	}

	log.Info("Starting clientapi")

	roomserverProducer, err := producers.NewRoomserverProducer(cfg.KafkaProducerURIs, cfg.ClientAPIOutputTopic)
	if err != nil {
		log.Panicf("Failed to setup kafka producers(%s): %s", cfg.KafkaProducerURIs, err)
	}

	queryAPI := api.NewRoomserverQueryAPIHTTP(cfg.RoomserverURL, nil)

	routing.Setup(http.DefaultServeMux, http.DefaultClient, cfg, roomserverProducer, queryAPI)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
