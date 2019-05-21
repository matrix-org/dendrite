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

package common

import (
	"context"
	"fmt"

	sarama "gopkg.in/Shopify/sarama.v1"
)

// A PartitionOffset is the offset into a partition of the input log.
type PartitionOffset struct {
	// The ID of the partition.
	Partition int32
	// The offset into the partition.
	Offset int64
}

// A PartitionStorer has the storage APIs needed by the consumer.
type PartitionStorer interface {
	// PartitionOffsets returns the offsets the consumer has reached for each partition.
	PartitionOffsets(ctx context.Context, topic string) ([]PartitionOffset, error)
	// SetPartitionOffset records where the consumer has reached for a partition.
	SetPartitionOffset(ctx context.Context, topic string, partition int32, offset int64) error
}

// A ContinualConsumer continually consumes logs even across restarts. It requires a PartitionStorer to
// remember the offset it reached.
type ContinualConsumer struct {
	// The kafkaesque topic to consume events from.
	// This is the name used in kafka to identify the stream to consume events from.
	Topic string
	// A kafkaesque stream consumer providing the APIs for talking to the event source.
	// The interface is taken from a client library for Apache Kafka.
	// But any equivalent event streaming protocol could be made to implement the same interface.
	Consumer sarama.Consumer
	// A thing which can load and save partition offsets for a topic.
	PartitionStore PartitionStorer
	// ProcessMessage is a function which will be called for each message in the log. Return an error to
	// stop processing messages. See ErrShutdown for specific control signals.
	ProcessMessage func(msg *sarama.ConsumerMessage) error
	// ShutdownCallback is called when ProcessMessage returns ErrShutdown, after the partition has been saved.
	// It is optional.
	ShutdownCallback func()
}

// ErrShutdown can be returned from ContinualConsumer.ProcessMessage to stop the ContinualConsumer.
var ErrShutdown = fmt.Errorf("shutdown")

// Start starts the consumer consuming.
// Starts up a goroutine for each partition in the kafka stream.
// Returns nil once all the goroutines are started.
// Returns an error if it can't start consuming for any of the partitions.
func (c *ContinualConsumer) Start() error {
	offsets := map[int32]int64{}

	partitions, err := c.Consumer.Partitions(c.Topic)
	if err != nil {
		return err
	}
	for _, partition := range partitions {
		// Default all the offsets to the beginning of the stream.
		offsets[partition] = sarama.OffsetOldest
	}

	storedOffsets, err := c.PartitionStore.PartitionOffsets(context.TODO(), c.Topic)
	if err != nil {
		return err
	}
	for _, offset := range storedOffsets {
		// We've already processed events from this partition so advance the offset to where we got to.
		// ConsumePartition will start streaming from the message with the given offset (inclusive),
		// so increment 1 to avoid getting the same message a second time.
		offsets[offset.Partition] = 1 + offset.Offset
	}

	var partitionConsumers []sarama.PartitionConsumer
	for partition, offset := range offsets {
		pc, err := c.Consumer.ConsumePartition(c.Topic, partition, offset)
		if err != nil {
			for _, p := range partitionConsumers {
				p.Close() // nolint: errcheck
			}
			return err
		}
		partitionConsumers = append(partitionConsumers, pc)
	}
	for _, pc := range partitionConsumers {
		go c.consumePartition(pc)
	}

	return nil
}

// consumePartition consumes the room events for a single partition of the kafkaesque stream.
func (c *ContinualConsumer) consumePartition(pc sarama.PartitionConsumer) {
	defer pc.Close() // nolint: errcheck
	for message := range pc.Messages() {
		msgErr := c.ProcessMessage(message)
		// Advance our position in the stream so that we will start at the right position after a restart.
		if err := c.PartitionStore.SetPartitionOffset(context.TODO(), c.Topic, message.Partition, message.Offset); err != nil {
			panic(fmt.Errorf("the ContinualConsumer failed to SetPartitionOffset: %s", err))
		}
		// Shutdown if we were told to do so.
		if msgErr == ErrShutdown {
			if c.ShutdownCallback != nil {
				c.ShutdownCallback()
			}
			return
		}
	}
}
