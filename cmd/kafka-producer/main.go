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
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/matrix-org/dendrite/common/basecomponent"

	sarama "gopkg.in/Shopify/sarama.v1"
)

const usage = `Usage: %s

Reads a list of newline separated messages from stdin and writes them to a single partition in kafka.

Arguments:

`

var (
	brokerList = flag.String("brokers", os.Getenv("KAFKA_PEERS"), "The comma separated list of brokers in the Kafka cluster. You can also set the KAFKA_PEERS environment variable")
	topic      = flag.String("topic", basecomponent.EnvParse("DENDRITE_KAFKAPROD_TOPIC", ""), "REQUIRED: the topic to produce to")
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
