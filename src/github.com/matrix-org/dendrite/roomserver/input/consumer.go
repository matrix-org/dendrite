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

// Package input contains the code that writes
package input

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// A ConsumerDatabase has the storage APIs needed by the consumer.
type ConsumerDatabase interface {
	RoomEventDatabase
	common.PartitionStorer
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
	ContinualConsumer common.ContinualConsumer
	// The database used to store the room events.
	DB       ConsumerDatabase
	Producer sarama.SyncProducer
	// The kafkaesque topic to output new room events to.
	// This is the name used in kafka to identify the stream to write events to.
	OutputRoomEventTopic string
	// The ErrorLogger for this consumer.
	// If left as nil then the consumer will panic when it encounters an error
	ErrorLogger ErrorLogger
	// If non-nil then the consumer will stop processing messages after this
	// many messages and will shutdown. Malformed messages are included in the count.
	StopProcessingAfter *int64
	// If not-nil then the consumer will call this to shutdown the server.
	ShutdownCallback func(reason string)
	// How many messages the consumer has processed.
	processed int64
}

// WriteOutputRoomEvent implements OutputRoomEventWriter
func (c *Consumer) WriteOutputRoomEvent(output api.OutputNewRoomEvent) error {
	var m sarama.ProducerMessage
	oe := api.OutputEvent{
		Type:         api.OutputTypeNewRoomEvent,
		NewRoomEvent: &output,
	}
	value, err := json.Marshal(oe)
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
	c.ContinualConsumer.ProcessMessage = c.processMessage
	c.ContinualConsumer.ShutdownCallback = c.shutdown
	return c.ContinualConsumer.Start()
}

func (c *Consumer) processMessage(message *sarama.ConsumerMessage) error {
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
	// Update the number of processed messages using atomic addition because it is accessed from multiple goroutines.
	processed := atomic.AddInt64(&c.processed, 1)
	// Check if we should stop processing.
	// Note that since we have multiple goroutines it's quite likely that we'll overshoot by a few messages.
	// If we try to stop processing after M message and we have N goroutines then we will process somewhere
	// between M and (N + M) messages because the N goroutines could all try to process what they think will be the
	// last message. We could be more careful here but this is good enough for getting rough benchmarks.
	if c.StopProcessingAfter != nil && processed >= int64(*c.StopProcessingAfter) {
		return common.ErrShutdown
	}
	return nil
}

func (c *Consumer) shutdown() {
	if c.ShutdownCallback != nil {
		c.ShutdownCallback(fmt.Sprintf("Stopping processing after %d messages", c.processed))
	}
}

// logError is a convenience method for logging errors.
func (c *Consumer) logError(message *sarama.ConsumerMessage, err error) {
	if c.ErrorLogger == nil {
		panic(err)
	}
	c.ErrorLogger.OnError(message, err)
}
