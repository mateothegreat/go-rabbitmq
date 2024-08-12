package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqprpc "github.com/0x4b53/amqp-rpc"
	"github.com/mateothegreat/go-multilog/multilog"
	"github.com/nvr-ai/go-types"
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

func RPC[R any](ctx context.Context, key types.MapKey, payload interface{}) (*R, error) {
	m, _ := json.Marshal(payload)
	request := amqprpc.NewRequest().WithRoutingKey(string(key)).WithBody(string(m)).WithTimeout(10 * time.Second)

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
