package rpc

import (
	"fmt"

	amqprpc "github.com/0x4b53/amqp-rpc"
	"github.com/mateothegreat/go-multilog/multilog"
)

var rpcClient *amqprpc.Client

func SetupClient(uri string) {
	rpcClient = amqprpc.NewClient(uri)
}

func SetupServer(uri string, routingKey string) {
	s := amqprpc.NewServer(uri)
	s.WithErrorLogger(func(format string, args ...interface{}) {
		multilog.Error("webrtc", "setup:witherrorlogger", map[string]interface{}{
			"message": fmt.Sprintf(format, args...),
		})
	})
	s.WithConsumeSettings(amqprpc.ConsumeSettings{
		QoSPrefetchCount: 1,
	})

	s.Bind(amqprpc.DirectBinding(routingKey, amqprpc.HandlerFunc(RouteRPC)))

	go s.ListenAndServe()
}
