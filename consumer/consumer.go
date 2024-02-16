package consumer

import (
	"log"

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

	p.Connection = connection

	return nil
}

func Consume(p *Consumer, queue string, ch chan<- *amqp.Delivery) error {
	log.Printf("consumer: starting consume on %s", queue)

	var err error

	p.Channel, err = p.Connection.Conn.Channel()

	if err != nil {
		return err
	}

	// defer p.Channel.Close()

	p.Tag = "test-tag"
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
		log.Printf("consumer.Consume: received a message: %s", d.Body)
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
