package naffka

import (
	"fmt"
	"log"
	"sync"
	"time"

	sarama "gopkg.in/Shopify/sarama.v1"
)

// Naffka is an implementation of the sarama kafka API designed to run within a
// single go process. It implements both the sarama.SyncProducer and the
// sarama.Consumer interfaces. This means it can act as a drop in replacement
// for kafka for testing or single instance deployment.
type Naffka struct {
	db          Database
	topicsMutex sync.Mutex
	topics      map[string]*topic
}

// New creates a new Naffka instance.
func New(db Database) (*Naffka, error) {
	n := &Naffka{db: db, topics: map[string]*topic{}}
	maxOffsets, err := db.MaxOffsets()
	if err != nil {
		return nil, err
	}
	for topicName, offset := range maxOffsets {
		n.topics[topicName] = &topic{
			topicName:  topicName,
			nextOffset: offset + 1,
		}
	}
	return n, nil
}

// A Message is used internally within naffka to store messages.
// It is converted to a sarama.ConsumerMessage when exposed to the
// public APIs to maintain API compatibility with sarama.
type Message struct {
	Offset    int64
	Key       []byte
	Value     []byte
	Timestamp time.Time
}

func (m *Message) consumerMessage(topic string) *sarama.ConsumerMessage {
	return &sarama.ConsumerMessage{
		Topic:     topic,
		Offset:    m.Offset,
		Key:       m.Key,
		Value:     m.Value,
		Timestamp: m.Timestamp,
	}
}

// A Database is used to store naffka messages.
// Messages are stored so that new consumers can access the full message history.
type Database interface {
	// StoreMessages stores a list of messages.
	// Every message offset must be unique within each topic.
	// Messages must be stored monotonically and contiguously for each topic.
	// So for a given topic the message with offset n+1 is stored after the
	// the message with offset n.
	StoreMessages(topic string, messages []Message) error
	// FetchMessages fetches all messages with an offset greater than but not
	// including startOffset and less than but not including endOffset.
	// The range of offsets requested must not overlap with those stored by a
	// concurrent StoreMessages. The message offsets within the requested range
	// are contigous. That is FetchMessage("foo", n, m) will only be called
	// once the messages between n and m have been stored by StoreMessages.
	// Every call must return at least one message. That is there must be at
	// least one message between the start and offset.
	FetchMessages(topic string, startOffset, endOffset int64) ([]Message, error)
	// MaxOffsets returns the maximum offset for each topic.
	MaxOffsets() (map[string]int64, error)
}

// SendMessage implements sarama.SyncProducer
func (n *Naffka) SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error) {
	err = n.SendMessages([]*sarama.ProducerMessage{msg})
	return msg.Partition, msg.Offset, err
}

// SendMessages implements sarama.SyncProducer
func (n *Naffka) SendMessages(msgs []*sarama.ProducerMessage) error {
	byTopic := map[string][]*sarama.ProducerMessage{}
	for _, msg := range msgs {
		byTopic[msg.Topic] = append(byTopic[msg.Topic], msg)
	}
	var topicNames []string
	for topicName := range byTopic {
		topicNames = append(topicNames, topicName)
	}

	now := time.Now()
	topics := n.getTopics(topicNames)
	for topicName := range byTopic {
		if err := topics[topicName].send(now, byTopic[topicName]); err != nil {
			return err
		}
	}
	return nil
}

func (n *Naffka) getTopics(topicNames []string) map[string]*topic {
	n.topicsMutex.Lock()
	defer n.topicsMutex.Unlock()
	result := map[string]*topic{}
	for _, topicName := range topicNames {
		t := n.topics[topicName]
		if t == nil {
			// If the topic doesn't already exist then create it.
			t = &topic{db: n.db, topicName: topicName}
			n.topics[topicName] = t
		}
		result[topicName] = t
	}
	return result
}

// Topics implements sarama.Consumer
func (n *Naffka) Topics() ([]string, error) {
	n.topicsMutex.Lock()
	defer n.topicsMutex.Unlock()
	var result []string
	for topic := range n.topics {
		result = append(result, topic)
	}
	return result, nil
}

// Partitions implements sarama.Consumer
func (n *Naffka) Partitions(topic string) ([]int32, error) {
	// Naffka stores a single partition per topic, so this always returns a single partition ID.
	return []int32{0}, nil
}

// ConsumePartition implements sarama.Consumer
func (n *Naffka) ConsumePartition(topic string, partition int32, offset int64) (sarama.PartitionConsumer, error) {
	if partition != 0 {
		return nil, fmt.Errorf("Unknown partition ID %d", partition)
	}
	topics := n.getTopics([]string{topic})
	return topics[topic].consume(offset), nil
}

// HighWaterMarks implements sarama.Consumer
func (n *Naffka) HighWaterMarks() map[string]map[int32]int64 {
	n.topicsMutex.Lock()
	defer n.topicsMutex.Unlock()
	result := map[string]map[int32]int64{}
	for topicName, topic := range n.topics {
		result[topicName] = map[int32]int64{
			0: topic.highwaterMark(),
		}
	}
	return result
}

// Close implements sarama.SyncProducer and sarama.Consumer
func (n *Naffka) Close() error {
	return nil
}

const channelSize = 1024

type partitionConsumer struct {
	topic    *topic
	messages chan *sarama.ConsumerMessage
	// Whether the consumer is ready for new messages or whether it
	// is catching up on historic messages.
	// Reads and writes to this field are proctected by the topic mutex.
	ready bool
}

// AsyncClose implements sarama.PartitionConsumer
func (c *partitionConsumer) AsyncClose() {
}

// Close implements sarama.PartitionConsumer
func (c *partitionConsumer) Close() error {
	// TODO: Add support for performing a clean shutdown of the consumer.
	return nil
}

// Messages implements sarama.PartitionConsumer
func (c *partitionConsumer) Messages() <-chan *sarama.ConsumerMessage {
	return c.messages
}

// Errors implements sarama.PartitionConsumer
func (c *partitionConsumer) Errors() <-chan *sarama.ConsumerError {
	// TODO: Add option to pass consumer errors to an errors channel.
	return nil
}

// HighWaterMarkOffset implements sarama.PartitionConsumer
func (c *partitionConsumer) HighWaterMarkOffset() int64 {
	return c.topic.highwaterMark()
}

// block writes the message to the consumer blocking until the consumer is ready
// to add the message to the channel. Once the message is successfully added to
// the channel it will catch up by pulling historic messsages from the database.
func (c *partitionConsumer) block(cmsg *sarama.ConsumerMessage) {
	c.messages <- cmsg
	c.catchup(cmsg.Offset)
}

// catchup reads historic messages from the database until the consumer has caught
// up on all the historic messages.
func (c *partitionConsumer) catchup(fromOffset int64) {
	for {
		// First check if we have caught up.
		caughtUp, nextOffset := c.topic.hasCaughtUp(c, fromOffset)
		if caughtUp {
			return
		}
		// Limit the number of messages we request from the database to be the
		// capacity of the channel.
		if nextOffset > fromOffset+int64(cap(c.messages)) {
			nextOffset = fromOffset + int64(cap(c.messages))
		}
		// Fetch the messages from the database.
		msgs, err := c.topic.db.FetchMessages(c.topic.topicName, fromOffset, nextOffset)
		if err != nil {
			// TODO: Add option to write consumer errors to an errors channel
			// as an alternative to logging the errors.
			log.Print("Error reading messages: ", err)
			// Wait before retrying.
			// TODO: Maybe use an exponentional backoff scheme here.
			// TODO: This timeout should take account of all the other goroutines
			// that might be doing the same thing. (If there are a 10000 consumers
			// then we don't want to end up retrying every millisecond)
			time.Sleep(10 * time.Second)
			continue
		}
		if len(msgs) == 0 {
			// This should only happen if the database is corrupted and has lost the
			// messages between the requested offsets.
			log.Fatalf("Corrupt database returned no messages between %d and %d", fromOffset, nextOffset)
		}

		// Pass the messages into the consumer channel.
		// Blocking each write until the channel has enough space for the message.
		for i := range msgs {
			c.messages <- msgs[i].consumerMessage(c.topic.topicName)
		}
		// Update our the offset for the next loop iteration.
		fromOffset = msgs[len(msgs)-1].Offset
	}
}

type topic struct {
	db         Database
	topicName  string
	mutex      sync.Mutex
	consumers  []*partitionConsumer
	nextOffset int64
}

func (t *topic) send(now time.Time, pmsgs []*sarama.ProducerMessage) error {
	var err error
	// Encode the message keys and values.
	msgs := make([]Message, len(pmsgs))
	for i := range msgs {
		if pmsgs[i].Key != nil {
			msgs[i].Key, err = pmsgs[i].Key.Encode()
			if err != nil {
				return err
			}
		}
		if pmsgs[i].Value != nil {
			msgs[i].Value, err = pmsgs[i].Value.Encode()
			if err != nil {
				return err
			}
		}
		pmsgs[i].Timestamp = now
		msgs[i].Timestamp = now
	}
	// Take the lock before assigning the offsets.
	t.mutex.Lock()
	defer t.mutex.Unlock()
	offset := t.nextOffset
	for i := range msgs {
		pmsgs[i].Offset = offset
		msgs[i].Offset = offset
		offset++
	}
	// Store the messages while we hold the lock.
	err = t.db.StoreMessages(t.topicName, msgs)
	if err != nil {
		return err
	}
	t.nextOffset = offset

	// Now notify the consumers about the messages.
	for i := range msgs {
		cmsg := msgs[i].consumerMessage(t.topicName)
		for _, c := range t.consumers {
			if c.ready {
				select {
				case c.messages <- cmsg:
				default:
					// The consumer wasn't ready to receive a message because
					// the channel buffer was full.
					// Fork a goroutine to send the message so that we don't
					// block sending messages to the other consumers.
					c.ready = false
					go c.block(cmsg)
				}
			}
		}
	}

	return nil
}

func (t *topic) consume(offset int64) *partitionConsumer {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	c := &partitionConsumer{
		topic: t,
	}
	// Handle special offsets.
	if offset == sarama.OffsetNewest {
		offset = t.nextOffset
	}
	if offset == sarama.OffsetOldest {
		offset = -1
	}
	c.messages = make(chan *sarama.ConsumerMessage, channelSize)
	t.consumers = append(t.consumers, c)
	// Start catching up on historic messages in the background.
	go c.catchup(offset)
	return c
}

func (t *topic) hasCaughtUp(c *partitionConsumer, offset int64) (bool, int64) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// Check if we have caught up while holding a lock on the topic so there
	// isn't a way for our check to race with a new message being sent on the topic.
	if offset+1 == t.nextOffset {
		// We've caught up, the consumer can now receive messages as they are
		// sent rather than fetching them from the database.
		c.ready = true
		return true, t.nextOffset
	}
	return false, t.nextOffset
}

func (t *topic) highwaterMark() int64 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.nextOffset
}
