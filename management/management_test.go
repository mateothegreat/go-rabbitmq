package management

import (
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/nvr-ai/go-rabbitmq/connections"
	"github.com/stretchr/testify/suite"
)

type ManagementTestSuite struct {
	suite.Suite
	SetupArgs  SetupArgs
	Connection *connections.Connection
}

func TestManagementSuite(t *testing.T) {
	suite.Run(t, new(ManagementTestSuite))
}

func (s *ManagementTestSuite) SetupSuite() {
	connection, err := connections.CreateConnection("amqp://guest:guest@localhost:5672/")
	s.NoError(err)
	s.Connection = connection

	exchanges := make([]Exchange, 0)

	for i := 0; i < 5; i++ {
		exchange := Exchange{
			Name:    faker.UUIDDigit(),
			Type:    "topic",
			Durable: true,
		}

		for j := 0; j < 5; j++ {
			exchange.Queues = append(exchange.Queues, Queue{
				Name:    faker.UUIDDigit(),
				Durable: true,
			})
		}

		exchanges = append(exchanges, exchange)
	}

	s.SetupArgs = SetupArgs{
		Exchanges: exchanges,
	}
}

func (s *ManagementTestSuite) TestSetup() {
	err := Setup(s.Connection, s.SetupArgs)
	s.NoError(err)
}

func (s *ManagementTestSuite) TestTearDown() {
	err := DeleteExchanges(s.Connection, s.SetupArgs.Exchanges)
	s.NoError(err)
}
