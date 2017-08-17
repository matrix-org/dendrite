package naffka

import (
	"testing"
	"time"

	sarama "gopkg.in/Shopify/sarama.v1"
)

func TestSendAndReceive(t *testing.T) {
	naffka, err := New(&MemoryDatabase{})
	if err != nil {
		t.Fatal(err)
	}
	producer := sarama.SyncProducer(naffka)
	consumer := sarama.Consumer(naffka)
	const topic = "testTopic"
	const value = "Hello, World"

	c, err := consumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
	if err != nil {
		t.Fatal(err)
	}

	message := sarama.ProducerMessage{
		Value: sarama.StringEncoder(value),
		Topic: topic,
	}

	if _, _, err = producer.SendMessage(&message); err != nil {
		t.Fatal(err)
	}

	var result *sarama.ConsumerMessage
	select {
	case result = <-c.Messages():
	case _ = <-time.NewTimer(10 * time.Second).C:
		t.Fatal("expected to receive a message")
	}

	if string(result.Value) != value {
		t.Fatalf("wrong value: wanted %q got %q", value, string(result.Value))
	}

	select {
	case result = <-c.Messages():
		t.Fatal("expected to only receive one message")
	default:
	}
}

func TestDelayedReceive(t *testing.T) {
	naffka, err := New(&MemoryDatabase{})
	if err != nil {
		t.Fatal(err)
	}
	producer := sarama.SyncProducer(naffka)
	consumer := sarama.Consumer(naffka)
	const topic = "testTopic"
	const value = "Hello, World"

	message := sarama.ProducerMessage{
		Value: sarama.StringEncoder(value),
		Topic: topic,
	}

	if _, _, err = producer.SendMessage(&message); err != nil {
		t.Fatal(err)
	}

	c, err := consumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
	if err != nil {
		t.Fatal(err)
	}

	var result *sarama.ConsumerMessage
	select {
	case result = <-c.Messages():
	case _ = <-time.NewTimer(10 * time.Second).C:
		t.Fatal("expected to receive a message")
	}

	if string(result.Value) != value {
		t.Fatalf("wrong value: wanted %q got %q", value, string(result.Value))
	}
}
