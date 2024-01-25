// Package messages contains structs and functions for working with RabbitMQ messages.
package messages

type Receive struct {
	Namespace string `json:"namespace"`
	Body      []byte `json:"body"`
	Status    string `json:"status"`
}
