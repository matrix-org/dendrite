package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/Shopify/sarama"
	"os"
	"strings"
)

const usage = `Usage: %s

Reads a list of newline separated messages from stdin and writes them to a single partition in kafka.

Arguments:

`

var (
	brokerList = flag.String("brokers", os.Getenv("KAFKA_PEERS"), "The comma separated list of brokers in the Kafka cluster. You can also set the KAFKA_PEERS environment variable")
	topic      = flag.String("topic", "", "REQUIRED: the topic to produce to")
	partition  = flag.Int("partition", 0, "The partition to produce to. All the messages will be written to this partition.")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *brokerList == "" {
		fmt.Fprintln(os.Stderr, "no -brokers specified. Alternatively, set the KAFKA_PEERS environment variable")
		os.Exit(1)
	}

	if *topic == "" {
		fmt.Fprintln(os.Stderr, "no -topic specified")
		os.Exit(1)
	}

	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true
	config.Producer.Partitioner = sarama.NewManualPartitioner

	producer, err := sarama.NewSyncProducer(strings.Split(*brokerList, ","), config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to open Kafka producer:", err)
		os.Exit(1)
	}
	defer func() {
		if err := producer.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to close Kafka producer cleanly:", err)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		message := &sarama.ProducerMessage{
			Topic:     *topic,
			Partition: int32(*partition),
			Value:     sarama.ByteEncoder(line),
		}
		if _, _, err := producer.SendMessage(message); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to send message:", err)
			os.Exit(1)
		}

	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}

}
