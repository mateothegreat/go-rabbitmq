package consumer

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/nvr-ai/go-rabbitmq/management"
	"github.com/nvr-ai/go-rabbitmq/messages"
	"github.com/nvr-ai/go-rabbitmq/producer"
	"github.com/nvr-ai/go-util/routines"
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
	Chan     chan *messages.Message[TestPayload]
	Manager  *management.Management
}

func TestConsumerSuite(t *testing.T) {
	suite.Run(t, new(ConsumerTestSuite))
}

func (s *ConsumerTestSuite) SetupSuite() {
	manager := &management.Management{}
	s.Manager = manager

	producer := &producer.Producer{}
	err := producer.Connect("amqp://guest:guest@localhost:5672/")
	s.NoError(err)
	s.Producer = producer

	consumer := &Consumer{}
	err = consumer.Connect("amqp://guest:guest@localhost:5672/", management.Exchange{
		Name:    "test-exchange",
		Type:    "topic",
		Durable: true,
		Queues: []management.Queue{
			{
				Name:    "test-queue",
				Durable: true,
			},
		},
	})
	s.NoError(err)

	s.Chan = make(chan *messages.Message[TestPayload], 1)

	err = s.Manager.Connect("amqp://guest:guest@localhost:5672/", management.SetupArgs{
		Exchanges: []management.Exchange{s.Exchange},
	})

	s.NoError(err)
}

func (s *ConsumerTestSuite) TearDownSuite() {
	err := s.Manager.DeleteExchanges([]management.Exchange{s.Exchange})
	s.NoError(err)
}

func (s *ConsumerTestSuite) TestConsume() {

	go func() {
		err := Consume(s.Consumer, "test-queue", s.Chan)
		s.NoError(err)
	}()

	if !routines.WaitForCondition(func() bool {
		queue, err := s.Manager.CreatePassiveQueue(s.Exchange.Queues[0])
		s.NoError(err)
		return queue.Messages == 0
	}, 3*time.Second, 100*time.Millisecond) {
		s.Fail("Queue can't be consumed yet")
	}

	message := &messages.Message[TestPayload]{
		Payload: TestPayload{
			Hello: "world",
			T:     "test",
		},
	}

	err := producer.Publish(s.Producer, context.Background(), "test-exchange", "test-queue", message)
	s.NoError(err)

	if !routines.WaitForCondition(func() bool {
		for {
			select {
			case msg := <-s.Chan:
				log.Printf("consumer: received message %s", msg.Payload.Hello)
				s.Equal(message.Payload.Hello, msg.Payload.Hello)
				return true

			case <-time.After(3 * time.Second):
				log.Printf("consumer: still waiting on the message...")
				return false
			}
		}
	}, 5*time.Second, 100*time.Millisecond) {
		s.Fail("Queue still has messages")
	}
}
