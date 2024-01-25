// Package messages contains structs and functions for working with RabbitMQ messages.
package messages

import (
	"encoding/json"
	"log"
)

// Message is a struct that represents a message to be sent to RabbitMQ.
type Payload struct {
	Namespace string `json:"namespace"`
	Body      string `json:"body"`
	Status    string `json:"status"`
}

// Marshal converts a Message to a JSON string.
func (m *Payload) Marshal() string {
	r, err := json.Marshal(m)

	if err != nil {
		log.Fatalf("Error marshalling message: %s", err)
	}

	return string(r)
}
