package main

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/input"
	"github.com/matrix-org/dendrite/roomserver/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	sarama "gopkg.in/Shopify/sarama.v1"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
)

var (
	database             = os.Getenv("DATABASE")
	kafkaURIs            = strings.Split(os.Getenv("KAFKA_URIS"), ",")
	inputRoomEventTopic  = os.Getenv("TOPIC_INPUT_ROOM_EVENT")
	outputRoomEventTopic = os.Getenv("TOPIC_OUTPUT_ROOM_EVENT")
	bindAddr             = os.Getenv("BIND_ADDRESS")
	// Shuts the roomserver down after processing a given number of messages.
	// This is useful for running benchmarks for seeing how quickly the server
	// can process a given number of messages.
	stopProcessingAfter = os.Getenv("STOP_AFTER")
)

func main() {
	db, err := storage.Open(database)
	if err != nil {
		panic(err)
	}

	kafkaConsumer, err := sarama.NewConsumer(kafkaURIs, nil)
	if err != nil {
		panic(err)
	}

	kafkaProducer, err := sarama.NewSyncProducer(kafkaURIs, nil)
	if err != nil {
		panic(err)
	}

	consumer := input.Consumer{
		Consumer:             kafkaConsumer,
		DB:                   db,
		Producer:             kafkaProducer,
		InputRoomEventTopic:  inputRoomEventTopic,
		OutputRoomEventTopic: outputRoomEventTopic,
	}

	if stopProcessingAfter != "" {
		count, err := strconv.ParseInt(stopProcessingAfter, 10, 64)
		if err != nil {
			panic(err)
		}
		consumer.StopProcessingAfter = &count
		consumer.ShutdownCallback = func(message string) {
			fmt.Println("Stopping roomserver", message)
			os.Exit(0)
		}
	}

	if err = consumer.Start(); err != nil {
		panic(err)
	}

	queryAPI := query.RoomserverQueryAPI{
		DB: db,
	}

	queryAPI.SetupHTTP(http.DefaultServeMux)

	fmt.Println("Started roomserver")

	// TODO: Implement clean shutdown.
	if err := http.ListenAndServe(bindAddr, nil); err != nil {
		panic(err)
	}
}
