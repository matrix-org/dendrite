package common

import (
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
	PartitionOffsets(topic string) ([]PartitionOffset, error)
	// SetPartitionOffset records where the consumer has reached for a partition.
	SetPartitionOffset(topic string, partition int32, offset int64) error
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

	storedOffsets, err := c.PartitionStore.PartitionOffsets(c.Topic)
	if err != nil {
		return err
	}
	for _, offset := range storedOffsets {
		// We've already processed events from this partition so advance the offset to where we got to.
		offsets[offset.Partition] = 1 + offset.Offset
	}

	var partitionConsumers []sarama.PartitionConsumer
	for partition, offset := range offsets {
		pc, err := c.Consumer.ConsumePartition(c.Topic, partition, offset)
		if err != nil {
			for _, p := range partitionConsumers {
				p.Close()
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
	defer pc.Close()
	for message := range pc.Messages() {
		msgErr := c.ProcessMessage(message)
		// Advance our position in the stream so that we will start at the right position after a restart.
		if err := c.PartitionStore.SetPartitionOffset(c.Topic, message.Partition, message.Offset); err != nil {
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
