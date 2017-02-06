// Package input contains the code that writes
package input

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// A ConsumerDatabase has the storage APIs needed by the consumer.
type ConsumerDatabase interface {
	RoomEventDatabase
	// PartitionOffsets returns the offsets the consumer has reached for each partition.
	PartitionOffsets(topic string) ([]types.PartitionOffset, error)
	// SetPartitionOffset records where the consumer has reached for a partition.
	SetPartitionOffset(topic string, partition int32, offset int64) error
}

// An ErrorLogger handles the errors encountered by the consumer.
type ErrorLogger interface {
	OnError(message *sarama.ConsumerMessage, err error)
}

// A Consumer consumes a kafkaesque stream of room events.
// The room events are supplied as api.InputRoomEvent structs serialised as JSON.
// The events should be valid matrix events.
// The events needed to authenticate the event should already be stored on the roomserver.
// The events needed to construct the state at the event should already be stored on the roomserver.
// If the event is not valid then it will be discarded and an error will be logged.
type Consumer struct {
	// A kafkaesque stream consumer providing the APIs for talking to the event source.
	// The interface is taken from a client library for Apache Kafka.
	// But any equivalent event streaming protocol could be made to implement the same interface.
	Consumer sarama.Consumer
	// The database used to store the room events.
	DB ConsumerDatabase
	// The kafkaesque topic to consume room events from.
	// This is the name used in kafka to identify the stream to consume events from.
	RoomEventTopic string
	// The ErrorLogger for this consumer.
	// If left as nil then the consumer will panic when it encounters an error
	ErrorLogger ErrorLogger
}

// Start starts the consumer consuming.
// Starts up a goroutine for each partition in the kafka stream.
// Returns nil once all the goroutines are started.
// Returns an error if it can't start consuming for any of the partitions.
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
func (c *Consumer) consumePartition(pc sarama.PartitionConsumer) {
	defer pc.Close()
	for message := range pc.Messages() {
		var input api.InputRoomEvent
		if err := json.Unmarshal(message.Value, &message.Value); err != nil {
			c.logError(message, err)
		} else {
			if err := processRoomEvent(c.DB, input); err != nil {
				c.logError(message, err)
			}
		}
		if err := c.DB.SetPartitionOffset(c.RoomEventTopic, message.Partition, message.Offset); err != nil {
			c.logError(message, err)
		}
	}
}

// logError is a convenience method for logging errors.
func (c *Consumer) logError(message *sarama.ConsumerMessage, err error) {
	if c.ErrorLogger == nil {
		panic(err)
	}
	c.ErrorLogger.OnError(message, err)
}
