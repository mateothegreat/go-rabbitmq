package types

import (
	"encoding/json"
	"time"
)

type InternalMessage[T any] struct {
	CorrelationID string    `json:"correlationId,omitempty"`
	Method        string    `json:"method"`
	Date          time.Time `json:"date"`
	Context       string    `json:"context,omitempty"`
	Data          T         `json:"data,omitempty"`
}

func (m *InternalMessage[T]) Marshal() ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (m *InternalMessage[T]) Unmarshal(b []byte) error {
	return json.Unmarshal(b, m)
}
