// Package input contains the code that writes
package input

import (
	"encoding/json"
	"fmt"
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
	DB       ConsumerDatabase
	Producer sarama.SyncProducer
	// The kafkaesque topic to consume room events from.
	// This is the name used in kafka to identify the stream to consume events from.
	InputRoomEventTopic string
	// The kafkaesque topic to output new room events to.
	// This is the name used in kafka to identify the stream to write events to.
	OutputRoomEventTopic string
	// The ErrorLogger for this consumer.
	// If left as nil then the consumer will panic when it encounters an error
	ErrorLogger ErrorLogger
	// If non-nil then the consumer will stop processing messages after this
	// many messages and will shutdown
	StopProcessingAfter *int
	// If not-nil then the consumer will call this to shutdown the server.
	ShutdownCallback func(reason string)
}

// WriteOutputRoomEvent implements OutputRoomEventWriter
func (c *Consumer) WriteOutputRoomEvent(output api.OutputRoomEvent) error {
	var m sarama.ProducerMessage
	value, err := json.Marshal(output)
	if err != nil {
		return err
	}
	m.Topic = c.OutputRoomEventTopic
	m.Key = sarama.StringEncoder("")
	m.Value = sarama.ByteEncoder(value)
	_, _, err = c.Producer.SendMessage(&m)
	return err
}

// Start starts the consumer consuming.
// Starts up a goroutine for each partition in the kafka stream.
// Returns nil once all the goroutines are started.
// Returns an error if it can't start consuming for any of the partitions.
func (c *Consumer) Start() error {
	offsets := map[int32]int64{}

	partitions, err := c.Consumer.Partitions(c.InputRoomEventTopic)
	if err != nil {
		return err
	}
	for _, partition := range partitions {
		// Default all the offsets to the beginning of the stream.
		offsets[partition] = sarama.OffsetOldest
	}

	storedOffsets, err := c.DB.PartitionOffsets(c.InputRoomEventTopic)
	if err != nil {
		return err
	}
	for _, offset := range storedOffsets {
		// We've already processed events from this partition so advance the offset to where we got to.
		offsets[offset.Partition] = offset.Offset
	}

	var partitionConsumers []sarama.PartitionConsumer
	for partition, offset := range offsets {
		pc, err := c.Consumer.ConsumePartition(c.InputRoomEventTopic, partition, offset)
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
	var processed int
	for message := range pc.Messages() {
		if c.StopProcessingAfter != nil && processed >= *c.StopProcessingAfter {
			if c.ShutdownCallback != nil {
				c.ShutdownCallback(fmt.Sprintf("Stopping processing after %d messages", processed))
			}
			return
		}
		var input api.InputRoomEvent
		if err := json.Unmarshal(message.Value, &input); err != nil {
			// If the message is invalid then log it and move onto the next message in the stream.
			c.logError(message, err)
		} else {
			if err := processRoomEvent(c.DB, c, input); err != nil {
				// If there was an error processing the message then log it and
				// move onto the next message in the stream.
				// TODO: If the error was due to a problem talking to the database
				// then we shouldn't move onto the next message and we should either
				// retry processing the message, or panic and kill ourselves.
				c.logError(message, err)
			}
		}
		// Advance our position in the stream so that we will start at the right position after a restart.
		if err := c.DB.SetPartitionOffset(c.InputRoomEventTopic, message.Partition, message.Offset); err != nil {
			c.logError(message, err)
		}
		processed++
	}
}

// logError is a convenience method for logging errors.
func (c *Consumer) logError(message *sarama.ConsumerMessage, err error) {
	if c.ErrorLogger == nil {
		panic(err)
	}
	c.ErrorLogger.OnError(message, err)
}
