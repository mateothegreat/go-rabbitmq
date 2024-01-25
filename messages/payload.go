// Package messages contains structs and functions for working with RabbitMQ messages.
package messages

import (
	"encoding/json"
	"log"
)

// Payload is a struct that represents a message to be sent to RabbitMQ.
type Payload struct {
	Namespace string      `json:"namespace"`
	Body      interface{} `json:"body"`
	Status    string      `json:"status"`
}

// Marshal converts a Message to a JSON string.
func (m *Payload) Marshal() []byte {
	r, err := json.Marshal(m)

	if err != nil {
		log.Fatalf("Error marshalling message: %s", err)
	}

	return r
}
