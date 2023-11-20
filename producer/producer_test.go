package producer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nvr-ai/go-rabbitmq/management"
	"github.com/nvr-ai/go-rabbitmq/messages"
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
}

func TestTenantSuite(t *testing.T) {
	suite.Run(t, new(ProducerTestSuite))
}

func (s *ProducerTestSuite) SetupSuite() {
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

	err = management.Setup(s.Producer.Connection, management.SetupArgs{
		Exchanges: []management.Exchange{s.Exchange},
	})

	s.NoError(err)
}

func (s *ProducerTestSuite) TearDownSuite() {
	println("TearDownSubTest")
}

func (s *ProducerTestSuite) TestNewConsumer() {

}

func (s *ProducerTestSuite) TestPublish() {
	err := s.Producer.Publish(context.Background(), s.Exchange.Name, s.Exchange.Queues[0].Name, &messages.Message{
		Payload: &TestPayload{Hello: "world", T: time.Now().String()},
	})
	s.NoError(err)
}
