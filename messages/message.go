package messages

type Message[T any] struct {
	Payload T `json:"payload"`
}
