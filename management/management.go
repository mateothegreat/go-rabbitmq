package management

import (
	"github.com/nvr-ai/go-rabbitmq/connections"
	amqp "github.com/rabbitmq/amqp091-go"
)

type SetupArgs struct {
	Exchanges []Exchange
}

type Management struct {
	Connection *connections.Connection
}

func (m *Management) Connect(uri string, args SetupArgs) error {
	config := amqp.Config{
		Vhost:      "/",
		Properties: amqp.NewConnectionProperties(),
	}
	config.Properties.SetClientConnectionName("producer-with-confirms")

	connection, err := connections.CreateConnection(uri)

	if err != nil {
		return err
	}

	m.Connection = connection

	err = m.CreateExchanges(args.Exchanges)

	if err != nil {
		return err
	}

	return nil
}

func (m *Management) CreateExchanges(exchanges []Exchange) error {
	for _, exchange := range exchanges {
		if err := m.Connection.Channel.ExchangeDeclare(exchange.Name, exchange.Type, exchange.Durable, true, false, false, nil); err != nil {
			return err
		}

		if err := m.CreateQueues(exchange); err != nil {
			return err
		}
	}

	return nil
}

func (m *Management) DeleteExchanges(exchanges []Exchange) error {
	for _, exchange := range exchanges {
		if err := m.DeleteQueues(exchange); err != nil {
			return err
		}

		if err := m.Connection.Channel.ExchangeDelete(exchange.Name, false, false); err != nil {
			return err
		}
	}

	return nil
}

func (m *Management) CreateQueue(exchange string, queue Queue) error {
	_, err := m.Connection.Channel.QueueDeclare(queue.Name, queue.Durable, true, false, false, nil)
	if err != nil {
		return err
	}

	if err := m.Connection.Channel.QueueBind(queue.Name, queue.Name, exchange, false, nil); err != nil {
		return err
	}

	return nil
}

func (m *Management) CreateQueues(exchange Exchange) error {
	for _, queue := range exchange.Queues {
		if err := m.CreateQueue(exchange.Name, queue); err != nil {
			return err
		}
	}

	return nil
}

func (m *Management) CreatePassiveQueue(queue Queue) (amqp.Queue, error) {
	q, err := m.Connection.Channel.QueueDeclarePassive(queue.Name, queue.Durable, false, false, false, nil)

	if err != nil {
		return amqp.Queue{}, err
	}

	return q, nil
}

func (m *Management) DeleteQueues(exchange Exchange) error {
	for _, queue := range exchange.Queues {
		if err := m.DeleteQueue(queue); err != nil {
			return err
		}
	}

	return nil
}

func (m *Management) DeleteQueue(queue Queue) error {
	if _, err := m.Connection.Channel.QueueDelete(queue.Name, false, false, false); err != nil {
		return err
	}

	return nil
}
