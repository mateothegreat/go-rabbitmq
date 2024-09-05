package rpc

import (
	"fmt"
	"time"

	amqprpcmw "github.com/0x4b53/amqp-rpc/middleware"

	amqprpc "github.com/0x4b53/amqp-rpc"
	"github.com/mateothegreat/go-multilog/multilog"
)

var amqprpcClient *amqprpc.Client

func Setup(uri string, routingKey string) error {
	amqprpcClient = amqprpc.NewClient(uri).WithTimeout(15 * time.Second)
	amqprpcClient = amqprpc.NewClient(uri).WithTimeout(15 * time.Second)
	amqprpcClient.WithErrorLogger(func(format string, args ...interface{}) {
		multilog.Error("amqprpc", fmt.Sprintf(format, args...), map[string]interface{}{})
	})
	amqprpcClient.WithDebugLogger(func(format string, args ...interface{}) {
		multilog.Debug("amqprpc", fmt.Sprintf(format, args...), map[string]interface{}{})
	})

	s := amqprpc.NewServer(uri)
	s.AddMiddleware(amqprpcmw.PanicRecoveryLogging(func(format string, args ...interface{}) {
		multilog.Error("webrtc", "setup:panicrecoverylogging", map[string]interface{}{
			"message": fmt.Sprintf(format, args...),
		})
	}))
	s.WithErrorLogger(func(format string, args ...interface{}) {
		multilog.Error("webrtc", "setup:witherrorlogger", map[string]interface{}{
			"message": fmt.Sprintf(format, args...),
		})
	})
	s.WithConsumeSettings(amqprpc.ConsumeSettings{
		QoSPrefetchCount: 1,
	})

	s.Bind(amqprpc.DirectBinding(routingKey, amqprpc.HandlerFunc(RouteRPC)))

	return nil
}

func GetClient() *amqprpc.Client {
	return amqprpcClient
}
