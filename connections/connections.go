package connections

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

type Connection struct {
	Uri     string
	Conn    *amqp.Connection
	Channel *amqp.Channel
}

func CreateConnection(uri string) (*Connection, error) {
	config := amqp.Config{
		Vhost:      "/",
		Properties: amqp.NewConnectionProperties(),
	}
	config.Properties.SetClientConnectionName("producer-with-confirms")

	conn, err := amqp.DialConfig(uri, config)

	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()

	if err != nil {
		return nil, err
	}

	return &Connection{
		Uri:     uri,
		Conn:    conn,
		Channel: ch,
	}, err
}
