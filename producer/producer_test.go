package producer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nvr-ai/go-rabbitmq/management"
	"github.com/nvr-ai/go-rabbitmq/messages"
	routines "github.com/nvr-ai/go-util/routines"
	"github.com/stretchr/testify/suite"
)

type TestPayload struct {
	Hello string `json:"hello"`
	T     string `json:"t"`
}

type ProducerTestSuite struct {
	suite.Suite
	Endpoint string
	Wg       sync.WaitGroup
	Producer *Producer
	Exchange management.Exchange
	Manager  *management.Management
}

func TestTenantSuite(t *testing.T) {
	suite.Run(t, new(ProducerTestSuite))
}

func (s *ProducerTestSuite) SetupSuite() {
	manager := &management.Management{}
	s.Manager = manager
	producer := &Producer{}

	err := producer.Connect("amqp://guest:guest@localhost:5672/")

	s.NoError(err)

	s.Producer = producer
	s.Exchange = management.Exchange{
		Name:    "test-exchange",
		Type:    "topic",
		Durable: true,
		Queues: []management.Queue{
			{
				Name:    "test-queue",
				Durable: true,
			},
		},
	}

	err = s.Manager.Connect("amqp://guest:guest@localhost:5672/", management.SetupArgs{
		Exchanges: []management.Exchange{s.Exchange},
	})

	s.NoError(err)
}

func (s *ProducerTestSuite) TearDownSuite() {
	err := s.Manager.DeleteExchanges([]management.Exchange{s.Exchange})
	s.NoError(err)
}

func (s *ProducerTestSuite) TestNewConsumer() {

}

func (s *ProducerTestSuite) TestPublish() {
	message := &messages.Message[TestPayload]{
		Payload: TestPayload{
			Hello: "world",
			T:     "test",
		},
	}
	err := Publish[TestPayload](s.Producer, context.Background(), s.Exchange.Name, s.Exchange.Queues[0].Name, message)
	s.NoError(err)

	if !routines.WaitForCondition(func() bool {
		queue, err := s.Manager.CreatePassiveQueue(s.Exchange.Queues[0])
		s.NoError(err)
		return queue.Messages == 1
	}, 3*time.Second, 100*time.Millisecond) {
		s.Fail("Queue still has messages")
	}
}
