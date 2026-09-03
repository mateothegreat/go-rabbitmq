package producer

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mateothegreat/go-rabbitmq/management"
	"github.com/stretchr/testify/suite"
)

// brokerURI is the broker these tests run against. `make test/setup` brings up
// a matching one via docker-compose.yaml.
const brokerURI = "amqp://rabbitmq:rabbitmq@localhost:5672"

// uri returns the broker to use, allowing RABBITMQ_URI to point the suite at
// something other than the compose broker.
func uri() string {
	if v := os.Getenv("RABBITMQ_URI"); v != "" {
		return v
	}

	return brokerURI
}

// requireBroker skips the suite when no broker is listening. Without it a
// missing broker costs the five minutes of connect backoff before failing.
func requireBroker(t *testing.T) {
	t.Helper()

	parsed, err := url.Parse(uri())
	if err != nil {
		t.Fatalf("invalid broker uri %q: %v", uri(), err)
	}

	host := parsed.Host
	if parsed.Port() == "" {
		host = net.JoinHostPort(host, "5672")
	}

	conn, err := net.DialTimeout("tcp", host, 500*time.Millisecond)
	if err != nil {
		t.Skipf("no broker reachable at %s (%v); run `make test/setup` to start one", host, err)
	}
	conn.Close()
}

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
	requireBroker(t)
	suite.Run(t, new(ProducerTestSuite))
}

func (s *ProducerTestSuite) SetupSuite() {
	s.Manager = &management.Management{}
	s.Exchange = management.Exchange{
		Name:    "producer-test-exchange",
		Type:    "topic",
		Durable: true,
		Queues: []management.Queue{
			{
				Name:    "producer-test-queue",
				Durable: true,
			},
		},
	}

	err := s.Manager.Connect(uri(), management.SetupArgs{
		Exchanges: []management.Exchange{s.Exchange},
	})
	s.Require().NoError(err)

	// Start from a known depth so the published count can be asserted.
	s.Require().NoError(s.Manager.DeleteQueues(s.Exchange))
	s.Require().NoError(s.Manager.CreateQueues(s.Exchange))

	producer := &Producer{}
	s.Require().NoError(producer.Connect(uri()))
	s.Producer = producer
}

func (s *ProducerTestSuite) TearDownSuite() {
	if s.Producer != nil {
		s.Producer.Close()
	}
	if s.Manager != nil {
		s.NoError(s.Manager.DeleteExchanges([]management.Exchange{s.Exchange}))
	}
}

// TestPublish publishes concurrently from many goroutines with no external
// locking and checks the broker acknowledged, and enqueued, every message. This
// is the case the readiness-token protocol could not serve: it capped the
// producer at one unconfirmed message and could wedge its confirmation handler.
func (s *ProducerTestSuite) TestPublish() {
	const numMessages = 1000

	queue := s.Exchange.Queues[0]

	before, err := s.Manager.CreatePassiveQueue(queue)
	s.Require().NoError(err)

	errs := make([]error, numMessages)

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(numMessages)
	for i := range numMessages {
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			errs[i] = s.Producer.Publish(ctx, s.Exchange.Name, queue.Name, []byte(fmt.Sprintf("Message %d", i)))
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)

	for i, err := range errs {
		s.NoErrorf(err, "publishing message %d", i)
	}

	// Publish only returns once the broker has confirmed, so the messages are
	// already enqueued by the time every goroutine has returned.
	after, err := s.Manager.CreatePassiveQueue(queue)
	s.Require().NoError(err)

	s.Equalf(before.Messages+numMessages, after.Messages,
		"queue depth went from %d to %d, expected %d more", before.Messages, after.Messages, numMessages)

	s.T().Logf("confirmed %d concurrent publishes in %s (%.0f msg/s)",
		numMessages, elapsed, float64(numMessages)/elapsed.Seconds())
}

// TestPublishAfterClose checks a closed producer reports the problem rather
// than blocking its caller.
func (s *ProducerTestSuite) TestPublishAfterClose() {
	p := &Producer{}
	s.Require().NoError(p.Connect(uri()))

	s.NoError(p.Publish(context.Background(), s.Exchange.Name, s.Exchange.Queues[0].Name, []byte("before close")))

	p.Close()
	p.Close() // Idempotent.

	s.ErrorIs(p.Publish(context.Background(), s.Exchange.Name, s.Exchange.Queues[0].Name, []byte("after close")), ErrNotConnected)
}

// TestReconnect checks Connect can replace a live channel and that the producer
// keeps publishing afterwards, without leaving the previous connection behind.
func (s *ProducerTestSuite) TestReconnect() {
	p := &Producer{}
	s.Require().NoError(p.Connect(uri()))
	defer p.Close()

	first := p.Connection

	s.Require().NoError(p.Connect(uri()))
	s.NotSame(first, p.Connection, "Connect should have installed a new connection")
	s.True(first.Conn.IsClosed(), "the superseded connection should have been closed")

	s.NoError(p.Publish(context.Background(), s.Exchange.Name, s.Exchange.Queues[0].Name, []byte("after reconnect")))
}
