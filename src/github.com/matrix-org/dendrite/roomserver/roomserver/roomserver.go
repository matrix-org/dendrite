package main

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/input"
	"github.com/matrix-org/dendrite/roomserver/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	sarama "gopkg.in/Shopify/sarama.v1"
	"net/http"
	"net/rpc"
	"os"
	"strings"
)

var (
	database             = os.Getenv("DATABASE")
	kafkaURIs            = strings.Split(os.Getenv("KAFKA_URIS"), ",")
	inputRoomEventTopic  = os.Getenv("TOPIC_INPUT_ROOM_EVENT")
	outputRoomEventTopic = os.Getenv("TOPIC_OUTPUT_ROOM_EVENT")
	bindAddr             = os.Getenv("BIND_ADDRESS")
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

	if err = consumer.Start(); err != nil {
		panic(err)
	}

	queryAPI := query.RoomserverQueryAPI{}

	if err = api.RegisterRoomserverQueryAPI(rpc.DefaultServer, &queryAPI); err != nil {
		panic(err)
	}

	rpc.HandleHTTP()

	fmt.Println("Started roomserver")

	// TODO: Implement clean shutdown.
	http.ListenAndServe(bindAddr, nil)
}
