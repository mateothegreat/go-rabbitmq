// Package types contains structs and functions for working with RabbitMQ messages.
package types

import (
	"encoding/json"
	"log"
)

// Message is a struct that represents a message to be sent to RabbitMQ.
type Message struct {
	Namespace string `json:"namespace"`
	Body      string `json:"body"`
}

// Marshal converts a Message to a JSON string.
func (m *Message) Marshal() string {
	r, err := json.Marshal(m)

	if err != nil {
		log.Fatalf("Error marshalling message: %s", err)
	}

	return string(r)
}
