// Package messages contains structs and functions for working with RabbitMQ messages.
package messages

type Receive[T any] struct {
	Namespace string `json:"namespace"`
	Body      T      `json:"body"`
	Status    string `json:"status"`
}
