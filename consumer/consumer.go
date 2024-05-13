package consumer

import (
	"log"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/nvr-ai/go-rabbitmq/connections"
	"github.com/nvr-ai/go-rabbitmq/management"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	Connection *connections.Connection
	Channel    *amqp.Channel
	Tag        string
	Done       chan error
	Manager    *management.Management
}

func (p *Consumer) Connect(uri string) error {
	operation := func() error {
		var err error
		p.Connection, err = connections.CreateConnection(uri)
		if err != nil {
			log.Printf("Failed to create connection: %v", err)
			return err
		}

		p.Channel, err = p.Connection.Conn.Channel()
		if err != nil {
			log.Printf("Failed to open channel: %v", err)
			return err
		}

		log.Printf("Consumer connected and channel established to %s", uri)
		return nil
	}

	// Retry with exponential backoff
	expBackOff := backoff.NewExponentialBackOff()
	expBackOff.MaxElapsedTime = 5 * time.Minute
	err := backoff.Retry(operation, expBackOff)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ after retries: %v", err)
	}

	return nil
}

func Consume(p *Consumer, queue string, ch chan<- *amqp.Delivery, name string) error {
	log.Printf("consumer[%s]: starting consume on %s", name, queue)

	var err error

	p.Channel, err = p.Connection.Conn.Channel()

	if err != nil {
		return err
	}

	// defer p.Channel.Close()

	p.Tag = name
	p.Channel.Qos(1, 0, false)

	deliveries, err := p.Channel.Consume(
		queue, // name
		p.Tag, // consumerTag,
		false, // noAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // arguments
	)

	if err != nil {
		return err
	}

	p.Done = make(chan error)

	// go handle(deliveries, p.Done)

	for d := range deliveries {
		log.Printf("consumer[%s]: received a message: %s", name, d.Body)
		ch <- &d
	}

	return nil
}

func (p *Consumer) Close() error {
	log.Printf("consumer: closing")

	err := p.Channel.Close()

	if err != nil {
		return err
	}

	err = p.Connection.Conn.Close()

	if err != nil {
		return err
	}

	return nil
}
