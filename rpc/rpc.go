package rpc

import (
	"context"
	"encoding/json"
	"time"

	amqprpc "github.com/0x4b53/amqp-rpc"
)

func RPC[R any](ctx context.Context, routingKey string, payload interface{}, timeout time.Duration) (*R, error) {
	m, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request := amqprpc.NewRequest().WithRoutingKey(routingKey).WithBody(string(m)).WithTimeout(timeout).WithContext(ctx)
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
