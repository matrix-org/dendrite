package naffka

import (
	"fmt"
	"sync"
)

// A MemoryDatabase stores the message history as arrays in memory.
// It can be used to run unit tests.
// If the process is stopped then any messages that haven't been
// processed by a consumer are lost forever.
type MemoryDatabase struct {
	topicsMutex sync.Mutex
	topics      map[string]*memoryDatabaseTopic
}

type memoryDatabaseTopic struct {
	messagesMutex sync.Mutex
	messages      []Message
}

func (t *memoryDatabaseTopic) addMessages(msgs []Message) error {
	t.messagesMutex.Lock()
	defer t.messagesMutex.Unlock()
	if int64(len(t.messages)) != msgs[0].Offset {
		return fmt.Errorf("message offset %d is not immediately after the previous offset %d", msgs[0].Offset, len(t.messages))
	}
	t.messages = append(t.messages, msgs...)
	return nil
}

// getMessages returns the current messages as a slice.
// This slice will have it's own copy of the length field so won't be affected
// by adding more messages in addMessages.
// The slice will share the same backing array with the slice we append new
// messages to.  It is safe to read the messages in the backing array since we
// only append to the slice.  It is not safe to write or append to the returned
// slice.
func (t *memoryDatabaseTopic) getMessages() []Message {
	t.messagesMutex.Lock()
	defer t.messagesMutex.Unlock()
	return t.messages
}

func (m *MemoryDatabase) getTopic(topicName string) *memoryDatabaseTopic {
	m.topicsMutex.Lock()
	defer m.topicsMutex.Unlock()
	result := m.topics[topicName]
	if result == nil {
		result = &memoryDatabaseTopic{}
		if m.topics == nil {
			m.topics = map[string]*memoryDatabaseTopic{}
		}
		m.topics[topicName] = result
	}
	return result
}

// StoreMessages implements Database
func (m *MemoryDatabase) StoreMessages(topic string, messages []Message) error {
	if err := m.getTopic(topic).addMessages(messages); err != nil {
		return err
	}
	return nil
}

// FetchMessages implements Database
func (m *MemoryDatabase) FetchMessages(topic string, startOffset, endOffset int64) ([]Message, error) {
	messages := m.getTopic(topic).getMessages()
	if endOffset > int64(len(messages)) {
		return nil, fmt.Errorf("end offset %d out of range %d", endOffset, len(messages))
	}
	if startOffset >= endOffset {
		return nil, fmt.Errorf("start offset %d greater than or equal to end offset %d", startOffset, endOffset)
	}
	if startOffset < -1 {
		return nil, fmt.Errorf("start offset %d less than -1", startOffset)
	}
	return messages[startOffset+1 : endOffset], nil
}

// MaxOffsets implements Database
func (m *MemoryDatabase) MaxOffsets() (map[string]int64, error) {
	m.topicsMutex.Lock()
	defer m.topicsMutex.Unlock()
	result := map[string]int64{}
	for name, t := range m.topics {
		result[name] = int64(len(t.getMessages())) - 1
	}
	return result, nil
}
