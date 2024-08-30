package rpc

import (
	"fmt"

	amqprpc "github.com/0x4b53/amqp-rpc"
	"github.com/mateothegreat/go-multilog/multilog"
)

var (
	handlers = make(map[string]amqprpc.HandlerFunc)
)

// AddHandler adds a handler to the broker.
//
// Arguments:
//   - name types.MapKey: the name of the handler
//   - handler amqprpc.HandlerFunc: the handler function
//
// Returns:
//   - error if failed to add handler
func AddHandler(name string, handler amqprpc.HandlerFunc) error {
	if _, exists := handlers[name]; exists {
		multilog.Error("broker", "add_handler", map[string]interface{}{
			"error": "handler already exists",
		})
		return fmt.Errorf("handler already exists")
	}

	handlers[name] = handler

	return nil
}
