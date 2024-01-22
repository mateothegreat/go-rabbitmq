package messages

import amqp "github.com/rabbitmq/amqp091-go"

type Message struct {
	Delivery amqp.Delivery
	Payload  []byte `json:"payload"`
}
