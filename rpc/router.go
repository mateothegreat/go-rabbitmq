package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	amqprpc "github.com/0x4b53/amqp-rpc"
	"github.com/nvr-ai/go-rabbitmq/producer"
	"github.com/nvr-ai/go-types"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/mateothegreat/go-multilog/multilog"
)

type RPCArgs struct {
	ResponseWriter *amqprpc.ResponseWriter
	Delivery       amqp.Delivery
}

type Handler interface {
	Handle(c context.Context, req types.ClientMessage[any], args RPCArgs)
}

type TypedHandler[T any] struct {
	handler func(c context.Context, req types.ClientMessage[T], args RPCArgs)
}

func (th *TypedHandler[T]) Handle(c context.Context, req types.ClientMessage[any], args RPCArgs) {
	dataType := reflect.TypeOf((*T)(nil)).Elem()
	multilog.Info("webrtc", "routerpc:handlerType", map[string]interface{}{
		"handlerType": dataType.Name(),
	})

	dataBytes, err := json.Marshal(req.Data)
	if err != nil {
		multilog.Error("webrtc", "routerpc:marshaldata", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	dataValue := reflect.New(dataType)
	err = json.Unmarshal(dataBytes, dataValue.Interface())
	if err != nil {
		multilog.Error("webrtc", "routerpc:unmarshaldata", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	req.Data = dataValue.Elem().Interface()

	typedReq := types.ClientMessage[T]{
		Method: req.Method,
		Data:   req.Data.(T),
	}

	th.handler(c, typedReq, args)
}

var (
	handlers = make(map[types.Method]Handler)
	Producer *producer.Producer
)

// AddHandler adds a handler to the broker.
func AddHandler[T any](name types.Method, handler func(c context.Context, req types.ClientMessage[T], args RPCArgs)) error {
	if _, exists := handlers[name]; exists {
		multilog.Error("webrtc", "addhandler:handleralreadyexists", map[string]interface{}{
			"error": "handler already exists",
		})
		return fmt.Errorf("handler already exists")
	}

	handlers[name] = &TypedHandler[T]{handler: handler}

	multilog.Debug("webrtc", "addhandler:handleradded", map[string]interface{}{
		"name": name,
	})

	return nil
}

func RouteRPC(c context.Context, rw *amqprpc.ResponseWriter, d amqp.Delivery) {
	var rawMessage map[string]json.RawMessage
	err := json.Unmarshal(d.Body, &rawMessage)
	if err != nil {
		multilog.Error("webrtc", "routerpc:unmarshal", map[string]interface{}{
			"error": err.Error(),
		})
		d.Ack(false)
		return
	}

	var method types.Method
	err = json.Unmarshal(rawMessage["method"], &method)
	if err != nil {
		multilog.Error("webrtc", "routerpc:unmarshalmethod", map[string]interface{}{
			"error": err.Error(),
		})
		d.Ack(false)
		return
	}

	if handler, exists := handlers[method]; exists {
		var msg types.ClientMessage[any]
		err := json.Unmarshal(d.Body, &msg)
		if err != nil {
			multilog.Error("webrtc", "routerpc:unmarshalclientmessage", map[string]interface{}{
				"error": err.Error(),
			})
			d.Ack(false)
			return
		}

		multilog.Info("webrtc", "routerpc:unmarshalclientmessage", map[string]interface{}{
			"message": msg,
		})

		handler.Handle(c, msg, RPCArgs{
			ResponseWriter: rw,
			Delivery:       d,
		})
		d.Ack(false)
	} else {
		multilog.Error("webrtc", "routerpc:handlernotfound", map[string]interface{}{
			"error":  "handler not found",
			"method": method,
		})
		d.Ack(false)
		return
	}
}
