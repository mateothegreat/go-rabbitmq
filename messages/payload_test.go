package messages

import (
	"log"
	"testing"
)

type Foo struct {
	Bar string `json:"bar"`
}

func TestA(t *testing.T) {

	payload := Payload{
		Namespace: "test",
		Body:      Foo{Bar: "baz"},
	}

	log.Printf("Payload: %+v", payload.Marshal())
}
