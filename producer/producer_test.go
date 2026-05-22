// package producer

// import (
// 	"context"
// 	"sync"
// 	"testing"
// 	"time"

// 	"github.com/mateothegreat/go-rabbitmq/management"
// 	"github.com/stretchr/testify/suite"
// )

// type TestPayload struct {
// 	Hello string `json:"hello"`
// 	T     string `json:"t"`
// }

// type ProducerTestSuite struct {
// 	suite.Suite
// 	Endpoint string
// 	Wg       sync.WaitGroup
// 	Producer *Producer
// 	Exchange management.Exchange
// 	Manager  *management.Management
// }

// func TestTenantSuite(t *testing.T) {
// 	suite.Run(t, new(ProducerTestSuite))
// }

// func (s *ProducerTestSuite) SetupSuite() {
// 	manager := &management.Management{}
// 	s.Manager = manager
// 	producer := &Producer{}

// 	err := producer.Connect("amqp://rabbitmq:Agby5kma0130@10.0.10.3:5672/")

// 	s.NoError(err)

// 	s.Producer = producer
// 	s.Exchange = management.Exchange{
// 		Name:    "test-exchange",
// 		Type:    "topic",
// 		Durable: true,
// 		Queues: []management.Queue{
// 			{
// 				Name:    "test-queue",
// 				Durable: true,
// 			},
// 		},
// 	}

// 	err = s.Manager.Connect("amqp://rabbitmq:Agby5kma0130@10.0.10.3:5672/", management.SetupArgs{
// 		Exchanges: []management.Exchange{s.Exchange},
// 	})

// 	s.NoError(err)
// }

// func (s *ProducerTestSuite) TearDownSuite() {
// 	err := s.Manager.DeleteExchanges([]management.Exchange{s.Exchange})
// 	s.NoError(err)
// }

// func (s *ProducerTestSuite) TestNewConsumer() {

// }

// func (s *ProducerTestSuite) TestPublish() {
// 	// ctx, cancel := context.WithCancel(context.Background())
// 	// defer cancel() // Ensure cancel is called to release resources

// 	// go func() {
// 	// 	for {
// 	// 		println(11)
// 	// 		err := s.Producer.Publish(ctx, s.Exchange.Name, s.Exchange.Queues[0].Name, []byte("test"), "test")
// 	// 		s.NoError(err)
// 	// 		println(22)
// 	// 	}
// 	// }()

// 	// // Wait for a few seconds before canceling the context
// 	// time.Sleep(time.Second * 3)

// 	// // Cancel the context to stop publishing
// 	// cancel()

// 	// // Wait for the goroutines in the producer to clean up
// 	// time.Sleep(time.Second * 2)
// 	// // if !routines.WaitForCondition(func() bool {
// 	// // 	queue, err := s.Manager.CreatePassiveQueue(s.Exchange.Queues[0])
// 	// // 	s.NoError(err)
// 	// // 	return queue.Messages == 1
// 	// // }, 3*time.Second, 100*time.Millisecond) {
// 	// // 	s.Fail("Queue still has messages")
// 	// // }
// 	// Number of producers to create
// 	numProducers := 1

// 	// Create a context with timeout for the test
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	// Channel to collect errors from goroutines
// 	errCh := make(chan error, numProducers)

// 	// Create and start multiple producers concurrently
// 	for i := 0; i < numProducers; i++ {
// 		go func(id int) {
// 			// Instantiate a new Producer
// 			p := &Producer{}

// 			// Connect to RabbitMQ
// 			err := p.Connect("amqp://rabbitmq:Agby5kma0130@10.0.10.3:5672/")
// 			if err != nil {
// 				errCh <- err
// 				return
// 			}

// 			// Publish a message
// 			err = p.Publish(ctx, "your_exchange", "your_key", []byte("your_message"), "your_client_name")
// 			if err != nil {
// 				errCh <- err
// 				return
// 			}
// 		}(i)
// 	}

//		// Wait for all goroutines to finish or timeout
//		for i := 0; i < numProducers; i++ {
//			select {
//			case err := <-errCh:
//				s.Failf("Error in goroutine: %v", err.Error())
//			case <-ctx.Done():
//				s.Failf("Test timeout: %v", ctx.Err().Error())
//				return
//			}
//		}
//	}
package producer

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mateothegreat/go-rabbitmq/management"
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

	err := producer.Connect("amqp://rabbitmq:rabbitmq@localhost:5672")

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

	err = s.Manager.Connect("amqp://rabbitmq:rabbitmq@localhost:5672", management.SetupArgs{
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
	// // ctx, cancel := context.WithCancel(context.Background())
	// // defer cancel() // Ensure cancel is called to release resources

	// // go func() {
	// // 	for {
	// // 		println(11)
	// // 		err := s.Producer.Publish(ctx, s.Exchange.Name, s.Exchange.Queues[0].Name, []byte("test"), "test")
	// // 		s.NoError(err)
	// // 		println(22)
	// // 	}
	// // }()

	// // // Wait for a few seconds before canceling the context
	// // time.Sleep(time.Second * 3)

	// // // Cancel the context to stop publishing
	// // cancel()

	// // // Wait for the goroutines in the producer to clean up
	// // time.Sleep(time.Second * 2)
	// // // if !routines.WaitForCondition(func() bool {
	// // // 	queue, err := s.Manager.CreatePassiveQueue(s.Exchange.Queues[0])
	// // // 	s.NoError(err)
	// // // 	return queue.Messages == 1
	// // // }, 3*time.Second, 100*time.Millisecond) {
	// // // 	s.Fail("Queue still has messages")
	// // // }
	// // Number of producers to create
	// numProducers := 1

	// // Create a context with timeout for the test
	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()

	// // Channel to collect errors from goroutines
	// errCh := make(chan error, numProducers)

	// // Create and start multiple producers concurrently
	// for i := 0; i < numProducers; i++ {
	// 	go func(id int) {
	// 		// Instantiate a new Producer
	// 		p := &Producer{}

	// 		// Connect to RabbitMQ
	// 		err := p.Connect("amqp://rabbitmq:Agby5kma0130@10.0.10.3:5672/")
	// 		if err != nil {
	// 			errCh <- err
	// 			return
	// 		}

	// 		// Publish a message
	// 		err = p.Publish(ctx, "your_exchange", "your_key", []byte("your_message"), "your_client_name")
	// 		if err != nil {
	// 			errCh <- err
	// 			return
	// 		}
	// 	}(i)
	// }

	// // Wait for all goroutines to finish or timeout
	// for i := 0; i < numProducers; i++ {
	// 	select {
	// 	case err := <-errCh:
	// 		s.Failf("Error in goroutine: %v", err.Error())
	// 	case <-ctx.Done():
	// 		s.Failf("Test timeout: %v", ctx.Err().Error())
	// 		return
	// 	}
	// }

	// // Instantiate producer
	// p := &Producer{}

	// err := p.Connect("amqp://rabbitmq:Agby5kma0130@10.0.10.3:5672/")
	// if err != nil {
	// 	s.Failf("Connect() returned an unexpected error: %v", err.Error())
	// }

	// for {
	// 	// Test successful publish
	// 	ctx := context.Background()
	// 	err = p.Publish(ctx, "exchange", "key", []byte("body"))
	// 	if err != nil {
	// 		s.Failf("Publish() returned an unexpected error: %v", err.Error())
	// 	}
	// }

	// println(1)
	// Number of messages to publish
	numMessages := 1000

	var wg sync.WaitGroup
	wg.Add(numMessages)

	for i := 0; i < numMessages; i++ {
		go func(i int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 115*time.Second)
			defer cancel()

			message := "Message " + strconv.Itoa(i)
			err := s.Producer.Publish(ctx, "", "test-queue", []byte(message))
			if err != nil {
				s.Failf("Failed to publish message %d: %v", strconv.Itoa(i), err)
			}
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()
}
