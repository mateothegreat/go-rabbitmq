package management

import (
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/suite"
)

type ManagementTestSuite struct {
	suite.Suite
	SetupArgs SetupArgs
	Manager   *Management
}

func TestManagementSuite(t *testing.T) {
	suite.Run(t, new(ManagementTestSuite))
}

func (s *ManagementTestSuite) SetupSuite() {
	manager := &Management{}
	s.Manager = manager

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
	err := s.Manager.Connect("amqp://guest:guest@localhost:5672/", s.SetupArgs)
	s.NoError(err)
}

func (s *ManagementTestSuite) TestTearDown() {
	err := s.Manager.DeleteExchanges(s.SetupArgs.Exchanges)
	s.NoError(err)
}
