package management

import (
	"github.com/nvr-ai/go-rabbitmq/connections"
)

type SetupArgs struct {
	Exchanges []Exchange
}

func Setup(connection *connections.Connection, args SetupArgs) error {
	err := CreateExchanges(connection, args.Exchanges)

	if err != nil {
		return err
	}

	return nil
}

func CreateExchanges(connection *connections.Connection, exchanges []Exchange) error {
	for _, exchange := range exchanges {
		if err := connection.Channel.ExchangeDeclare(exchange.Name, exchange.Type, exchange.Durable, true, false, false, nil); err != nil {
			return err
		}

		if err := CreateQueues(connection, exchange); err != nil {
			return err
		}
	}

	return nil
}

func DeleteExchanges(connection *connections.Connection, exchanges []Exchange) error {
	for _, exchange := range exchanges {
		if err := DeleteQueues(connection, exchange); err != nil {
			return err
		}

		if err := connection.Channel.ExchangeDelete(exchange.Name, false, false); err != nil {
			return err
		}
	}

	return nil
}
func CreateQueues(connection *connections.Connection, exchange Exchange) error {
	for _, queue := range exchange.Queues {
		if _, err := connection.Channel.QueueDeclare(queue.Name, queue.Durable, true, false, false, nil); err != nil {
			return err
		}
		if err := connection.Channel.QueueBind(queue.Name, queue.Name, exchange.Name, false, nil); err != nil {
			return err
		}
	}

	return nil
}

func DeleteQueues(connection *connections.Connection, exchange Exchange) error {
	for _, queue := range exchange.Queues {
		if _, err := connection.Channel.QueueDelete(queue.Name, false, false, false); err != nil {
			return err
		}
	}

	return nil
}
