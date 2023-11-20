package consumer

import (
	"encoding/json"
	"log"

	"github.com/nvr-ai/go-rabbitmq/connections"
	"github.com/nvr-ai/go-rabbitmq/messages"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	Connection *connections.Connection
	Channel    *amqp.Channel
	Tag        string
	Done       chan error
}

func (p *Consumer) Connect(uri string) error {
	log.Printf("consumer: dialing %s", uri)

	config := amqp.Config{
		Vhost:      "/",
		Properties: amqp.NewConnectionProperties(),
	}
	config.Properties.SetClientConnectionName("producer-with-confirms")

	connection, err := connections.CreateConnection(uri)

	if err != nil {
		return err
	}

	// defer connection.Conn.Close()

	p.Connection = connection

	return nil
}

func Consume[T any](p *Consumer, queue string, ch chan<- *messages.Message[T]) error {
	log.Printf("consumer: starting consume on %s", queue)

	var err error

	p.Channel, err = p.Connection.Conn.Channel()

	if err != nil {
		return err
	}

	// defer p.Channel.Close()

	p.Tag = "test-tag"

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
		log.Printf("consumer.Consume: received a message: %s", d.Body)

		message := &messages.Message[T]{}

		err := json.Unmarshal(d.Body, message)

		if err != nil {
			d.Ack(false)
			return err
		}

		ch <- message
		println("asdf")
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

func handle(deliveries <-chan amqp.Delivery, done chan error) {
	for d := range deliveries {
		log.Printf(
			"consumer: received a message: %s",
			d.Body,
		)
		d.Ack(false)
	}
	done <- nil
}
