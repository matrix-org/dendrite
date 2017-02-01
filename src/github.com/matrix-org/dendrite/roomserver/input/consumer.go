// Package input contains the code that writes
package input

import (
	"github.com/matrix-org/dendrite/roomserver/types"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// A ConsumerDatabase has the storage APIs needed by the consumer.
type ConsumerDatabase interface {
	// PartitionOffsets returns the offsets the consumer has reached for each partition.
	PartitionOffsets(topic string) ([]types.PartitionOffset, error)
	// SetPartitionOffset records where the consumer has reached for a partition.
	SetPartitionOffset(topic string, partition int32, offset int64) error
}

// An ErrorHandler handles the errors encountered by the consumer.
type ErrorHandler interface {
	OnError(err error)
}

// A Consumer consumes a kafkaesque stream of room events.
type Consumer struct {
	// A kafkaesque stream consumer.
	Consumer sarama.Consumer
	// The database used to store the room events.
	DB ConsumerDatabase
	// The kafkaesque topic to consume room events from.
	RoomEventTopic string
	// The ErrorHandler for this consumer.
	// If left as nil then the consumer will panic with that error.
	ErrorHandler ErrorHandler
}

// Start starts the consumer consuming.
func (c *Consumer) Start() error {
	offsets := map[int32]int64{}

	partitions, err := c.Consumer.Partitions(c.RoomEventTopic)
	if err != nil {
		return err
	}
	for _, partition := range partitions {
		// Default all the offsets to the beginning of the stream.
		offsets[partition] = sarama.OffsetOldest
	}

	storedOffsets, err := c.DB.PartitionOffsets(c.RoomEventTopic)
	if err != nil {
		return err
	}
	for _, offset := range storedOffsets {
		// We've already processed events from this partition so advance the offset to where we got to.
		offsets[offset.Partition] = offset.Offset
	}

	var partitionConsumers []sarama.PartitionConsumer
	for partition, offset := range offsets {
		pc, err := c.Consumer.ConsumePartition(c.RoomEventTopic, partition, offset)
		if err != nil {
			for _, pc := range partitionConsumers {
				pc.Close()
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
func (c *Consumer) consumePartition(pc sarama.PartitionConsumer) {
	for message := range pc.Messages() {
		// Do stuff with message.
		if err := c.DB.SetPartitionOffset(c.RoomEventTopic, message.Partition, message.Offset); err != nil {
			c.handleError(message, err)
		}
	}
}

// handleError is a convenience method for handling errors.
func (c *Consumer) handleError(message *sarama.ConsumerMessage, err error) {
	if c.ErrorHandler == nil {
		panic(err)
	}
	c.ErrorHandler.OnError(err)
}
