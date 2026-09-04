package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	amqprpc "github.com/0x4b53/amqp-rpc"
	"github.com/mateothegreat/go-rabbitmq/producer"
	"github.com/mateothegreat/go-rabbitmq/types"
	"github.com/streadway/amqp"

	"github.com/mateothegreat/multilog"
)

type RPCArgs struct {
	ResponseWriter *amqprpc.ResponseWriter
	Delivery       amqp.Delivery
}

type Handler interface {
	Handle(req types.InternalMessage[any], args RPCArgs)
}

type TypedHandler[T any] struct {
	handler func(req types.InternalMessage[T], args RPCArgs)
}

func (th *TypedHandler[T]) Handle(req types.InternalMessage[any], args RPCArgs) {
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

	typedReq := types.InternalMessage[T]{
		CorrelationID: req.CorrelationID,
		Date:          req.Date,
		Context:       req.Context,
		Method:        req.Method,
		Data:          req.Data.(T),
	}

	th.handler(typedReq, args)
}

var (
	handlers = make(map[string]Handler)
	Producer *producer.Producer
)

// AddHandler adds a handler to the broker.
func AddHandler[T any](name string, handler func(req types.InternalMessage[T], args RPCArgs)) error {
	if _, exists := handlers[name]; exists {
		multilog.Error("webrtc", "addhandler:handleralreadyexists", map[string]interface{}{
			"error": "handler already exists",
		})
		return fmt.Errorf("handler already exists")
	}

	handlers[name] = &TypedHandler[T]{
		handler: handler,
	}

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

	var method string
	err = json.Unmarshal(rawMessage["method"], &method)
	if err != nil {
		multilog.Error("webrtc", "routerpc:unmarshalmethod", map[string]interface{}{
			"error": err.Error(),
		})
		d.Ack(false)
		return
	}

	if handler, exists := handlers[method]; exists {
		var msg types.InternalMessage[any]
		err := json.Unmarshal(d.Body, &msg)
		if err != nil {
			multilog.Error("webrtc", "routerpc:unmarshalInternalMessage", map[string]interface{}{
				"error": err.Error(),
			})
			d.Ack(false)
			return
		}
		handler.Handle(msg, RPCArgs{
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
