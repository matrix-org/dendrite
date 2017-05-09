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

package test

import (
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// KafkaExecutor executes kafka scripts.
type KafkaExecutor struct {
	// The location of Zookeeper. Typically this is `localhost:2181`.
	ZookeeperURI string
	// The directory where Kafka is installed to. Used to locate kafka scripts.
	KafkaDirectory string
	// The location of the Kafka logs. Typically this is `localhost:9092`.
	KafkaURI string
	// Where stdout and stderr should be written to. Typically this is `os.Stderr`.
	OutputWriter io.Writer
}

// CreateTopic creates a new kafka topic. This is created with a single partition.
func (e *KafkaExecutor) CreateTopic(topic string) error {
	cmd := exec.Command(
		filepath.Join(e.KafkaDirectory, "bin", "kafka-topics.sh"),
		"--create",
		"--zookeeper", e.ZookeeperURI,
		"--replication-factor", "1",
		"--partitions", "1",
		"--topic", topic,
	)
	cmd.Stdout = e.OutputWriter
	cmd.Stderr = e.OutputWriter
	return cmd.Run()
}

// WriteToTopic writes data to a kafka topic.
func (e *KafkaExecutor) WriteToTopic(topic string, data []string) error {
	cmd := exec.Command(
		filepath.Join(e.KafkaDirectory, "bin", "kafka-console-producer.sh"),
		"--broker-list", e.KafkaURI,
		"--topic", topic,
	)
	cmd.Stdout = e.OutputWriter
	cmd.Stderr = e.OutputWriter
	cmd.Stdin = strings.NewReader(strings.Join(data, "\n"))
	return cmd.Run()
}

// DeleteTopic deletes a given kafka topic if it exists.
func (e *KafkaExecutor) DeleteTopic(topic string) error {
	cmd := exec.Command(
		filepath.Join(e.KafkaDirectory, "bin", "kafka-topics.sh"),
		"--delete",
		"--if-exists",
		"--zookeeper", e.ZookeeperURI,
		"--topic", topic,
	)
	cmd.Stderr = e.OutputWriter
	cmd.Stdout = e.OutputWriter
	return cmd.Run()
}
