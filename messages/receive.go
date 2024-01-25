// Package messages contains structs and functions for working with RabbitMQ messages.
package messages

type Receive struct {
	Namespace string  `json:"namespace"`
	Body      []uint8 `json:"body"`
	Status    string  `json:"status"`
}
