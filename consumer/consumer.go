package consumer

import (
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/mateothegreat/go-rabbitmq/connections"
	"github.com/mateothegreat/go-rabbitmq/management"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	Connection *connections.Connection
	Channel    *amqp.Channel
	Tag        string
	Manager    *management.Management
}
type ConsumerArgs struct {
	Queue         string
	Name          string
	AutoAck       bool
	Exclusive     bool
	NoLocal       bool
	NoWait        bool
	PrefetchSize  int
	PrefetchCount int
	Global        bool
	Arguments     map[string]any
}

func (p *Consumer) Connect(uri string) error {
	operation := func() error {
		var err error
		p.Connection, err = connections.CreateConnection(uri)
		if err != nil {
			return err
		}

		p.Channel, err = p.Connection.Conn.Channel()
		if err != nil {
			return err
		}

		return nil
	}

	// Retry with exponential backoff
	expBackOff := backoff.NewExponentialBackOff()
	expBackOff.MaxElapsedTime = 5 * time.Minute
	err := backoff.Retry(operation, expBackOff)
	if err != nil {
		return err
	}

	return nil
}

func (p *Consumer) Consume(args ConsumerArgs) (<-chan amqp.Delivery, error) {
	var err error
	p.Channel, err = p.Connection.Conn.Channel()
	if err != nil {
		return nil, err
	}

	p.Tag = args.Name
	p.Channel.Qos(args.PrefetchSize, args.PrefetchCount, args.Global)

	deliveries, err := p.Channel.Consume(
		args.Queue,     // name
		p.Tag,          // consumerTag,
		args.AutoAck,   // noAck
		args.Exclusive, // exclusive
		args.NoLocal,   // noLocal
		args.NoWait,    // noWait
		args.Arguments, // arguments
	)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (p *Consumer) Close() error {
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
