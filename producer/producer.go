package producer

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nvr-ai/go-rabbitmq/connections"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	Connection *connections.Connection
	Channel    *amqp.Channel
	exitCh     chan struct{}
}

func (p *Producer) Connect(uri string) error {
	log.Printf("producer: dialing %s", uri)

	connection, err := connections.CreateConnection(uri)
	if err != nil {
		return err
	}

	p.Connection = connection

	p.Channel, err = p.Connection.Conn.Channel()
	if err != nil {
		return err
	}

	setupCloseHandler(p.exitCh)
	return nil
}

func setupCloseHandler(exitCh chan struct{}) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Printf("close handler: Ctrl+C pressed in Terminal")
		close(exitCh)
	}()
}

func (p *Producer) Publish(ctx context.Context, exchange string, key string, body []byte, name string) error {
	if p.exitCh == nil {
		p.exitCh = make(chan struct{})
		setupCloseHandler(p.exitCh)
	}

	config := amqp.Config{
		Vhost:      "/",
		Properties: amqp.NewConnectionProperties(),
	}
	config.Properties.SetClientConnectionName(name)

	// Reliable publisher confirms require confirm.select support from the
	// connection.
	if err := p.Connection.Channel.Confirm(false); err != nil {
		return err
	}

	// for {
	log.Printf("producer: publishing %dB body (%q)", len(body), body)

	_, err := p.Connection.Channel.PublishWithDeferredConfirmWithContext(
		ctx,
		exchange,
		key,
		true,
		false,
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			DeliveryMode:    amqp.Persistent,
			Priority:        0,
			AppId:           "sequential-producer",
			Body:            body,
		},
	)
	if err != nil {
		return err
	}

	return nil
	// select {
	// case <-ctx.Done():
	// 	log.Println("producer: context canceled, stopping publish")
	// 	return ctx.Err()
	// case <-dConfirmation.Done():
	// 	log.Println("producer: confirmed delivery")
	// }
	// }
}
