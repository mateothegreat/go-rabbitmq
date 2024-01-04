package test

// import (
// 	"context"
// 	"testing"

// 	"github.com/nvr-ai/go-rabbitmq/producer"
// 	"github.com/stretchr/testify/suite"
// 	"github.com/testcontainers/testcontainers-go"
// 	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
// )

// type ProducerTestSuite struct {
// 	suite.Suite
// 	Endpoint string
// }

// func TestTenantSuite(t *testing.T) {
// 	suite.Run(t, new(ProducerTestSuite))
// }

// // func (s *ProducerTestSuite) SetupSuite() {
// // 	println("SetupSuite")
// // }

// // func (s *ProducerTestSuite) TearDownSubTest() {
// // 	println("TearDownSubTest")
// // }

// func (s *ProducerTestSuite) TestNewConsumer() {
// 	ctx := context.Background()

// 	rabbitmqContainer, err := rabbitmq.RunContainer(ctx,
// 		testcontainers.WithImage("rabbitmq:3.10-management"),
// 		rabbitmq.WithAdminUsername("guest"),
// 		rabbitmq.WithAdminPassword("guest"),
// 	)

// 	if err != nil {
// 		s.T().Error(err)
// 	}

// 	// Clean up the container
// 	defer func() {
// 		if err := rabbitmqContainer.Terminate(ctx); err != nil {
// 			s.T().Error(err)
// 		}
// 	}()

// 	amqpUrl, err := rabbitmqContainer.AmqpURL(ctx)
// 	if err != nil {
// 		s.T().Error(err)
// 	}

// 	println("RabbitMQ running at %s", amqpUrl)

// 	producer := &producer.Producer{}
// 	producer.Connect(amqpUrl)
// }
