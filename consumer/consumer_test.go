package consumer

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/nvr-ai/go-rabbitmq/management"
	"github.com/nvr-ai/go-rabbitmq/producer"
	"github.com/nvr-ai/go-util/routines"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
)

type TestPayload struct {
	Hello string `json:"hello"`
	T     string `json:"t"`
}

type ConsumerTestSuite struct {
	suite.Suite
	Endpoint string
	Wg       sync.WaitGroup
	Consumer *Consumer
	Producer *producer.Producer
	Exchange management.Exchange
	Manager  *management.Management
}

func TestConsumerSuite(t *testing.T) {
	suite.Run(t, new(ConsumerTestSuite))
}

func (s *ConsumerTestSuite) SetupSuite() {
	manager := &management.Management{}
	s.Manager = manager
	s.Exchange = management.Exchange{
		Name:    "test-exchange",
		Type:    "topic",
		Durable: false,
		Queues: []management.Queue{
			{
				Name:    "test-queue",
				Durable: false,
			},
		},
	}
	producer := &producer.Producer{}
	err := producer.Connect("amqp://rabbitmq:rabbitmq@localhost:5672/")
	s.NoError(err)
	s.Producer = producer

	consumer := &Consumer{}
	err = consumer.Connect("amqp://rabbitmq:rabbitmq@localhost:5672/")
	s.NoError(err)
	s.Consumer = consumer

	err = s.Manager.Connect("amqp://rabbitmq:rabbitmq@localhost:5672/", management.SetupArgs{
		Exchanges: []management.Exchange{s.Exchange},
	})

	s.NoError(err)
}

func (s *ConsumerTestSuite) TestConsume() {
	var err error
	var ch <-chan amqp091.Delivery

	go func() {
		ch, err = s.Consumer.Consume(ConsumerArgs{
			Queue: "test-queue",
		})
		s.NoError(err)
	}()

	if !routines.WaitForCondition(func() bool {
		queue, err := s.Manager.CreatePassiveQueue(s.Exchange.Queues[0])
		s.NoError(err)
		return queue.Messages == 0
	}, 3*time.Second, 100*time.Millisecond) {
		s.Fail("Queue can't be consumed yet")
	}

	err = s.Producer.Publish(context.Background(), "test-exchange", "test-queue", []byte("hello"))
	s.NoError(err)

	if !routines.WaitForCondition(func() bool {
		for {
			select {
			case msg := <-ch:
				log.Printf("consumer: received message %s", msg.Body)
				if ackErr := msg.Ack(false); ackErr != nil {
					log.Printf("consumer: error acking message: %s", ackErr)
					s.Fail("Error acking message")
					return false
				}
				s.Equal("hello", string(msg.Body))
				return true
			case <-time.After(3 * time.Second):
				log.Printf("consumer: still waiting on the message...")
				return false
			}
		}
	}, 5*time.Second, 100*time.Millisecond) {
		s.Fail("Queue still has messages")
	}

	err = s.Consumer.Close()
	s.NoError(err)
}
