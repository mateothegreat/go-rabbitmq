package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqprpc "github.com/0x4b53/amqp-rpc"
	"github.com/mateothegreat/go-multilog/multilog"
)

var amqprpcClient *amqprpc.Client

func Setup(uri string) {
	amqprpcClient = amqprpc.NewClient(uri).WithTimeout(15 * time.Second)
	amqprpcClient = amqprpc.NewClient(uri).WithTimeout(15 * time.Second)
	amqprpcClient.WithErrorLogger(func(format string, args ...interface{}) {
		multilog.Error("amqprpc", fmt.Sprintf(format, args...), map[string]interface{}{})
	})
	amqprpcClient.WithDebugLogger(func(format string, args ...interface{}) {
		multilog.Debug("amqprpc", fmt.Sprintf(format, args...), map[string]interface{}{})
	})
}

func GetClient() *amqprpc.Client {
	return amqprpcClient
}

func RPC[R any](ctx context.Context, routingKey string, payload interface{}) (*R, error) {
	m, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request := amqprpc.NewRequest().WithRoutingKey(routingKey).WithBody(string(m)).WithTimeout(10 * time.Second)
	response, err := GetClient().Send(request)
	if err != nil {
		return nil, err
	}
	var r R
	err = json.Unmarshal([]byte(response.Body), &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
