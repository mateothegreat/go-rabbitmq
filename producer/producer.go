package producer

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nvr-ai/go-rabbitmq/connections"
	"github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	Connection   *connections.Connection
	Channel      *amqp091.Channel
	exitCh       chan struct{}
	confirms     chan amqp091.Confirmation
	confirmsDone chan struct{}
	publishOk    chan struct{}
}

func (p *Producer) Connect(uri string) error {

	var err error
	p.Connection, err = connections.CreateConnection(uri)
	if err != nil {
		return err
	}

	p.Channel, err = p.Connection.Conn.Channel()
	if err != nil {
		return err
	}

	if err := p.Channel.Confirm(false); err != nil {
		return err
	}

	p.exitCh = make(chan struct{})
	p.confirms = make(chan amqp091.Confirmation, 1)
	p.confirmsDone = make(chan struct{})
	p.publishOk = make(chan struct{}, 1) // Signal initial readiness

	log.Printf("producer: dialed %s", uri)

	// Start listening for confirmations.
	confirmChan := p.Channel.NotifyPublish(make(chan amqp091.Confirmation, 1))
	go p.handleConfirms(confirmChan)
	p.publishOk <- struct{}{} // Signal initial readiness

	return nil
}

func (p *Producer) handleConfirms(confirmChan <-chan amqp091.Confirmation) {
	for {
		select {
		case confirm := <-confirmChan:
			if confirm.Ack {
				log.Printf("Message confirmed with delivery tag: %d", confirm.DeliveryTag)
			} else {
				log.Printf("Message nack with delivery tag: %d", confirm.DeliveryTag)
			}
			// Re-signal readiness after each confirmation
			p.signalPublishOk()
		case <-p.exitCh:
			log.Println("Confirmation handler exiting")
			return
		}
	}
}

func (p *Producer) signalPublishOk() {
	p.publishOk <- struct{}{}
	// select {
	// case p.publishOk <- struct{}{}:
	// 	log.Println("Signaled readiness to publish")
	// 	// default:
	// 	// 	log.Println("Publish readiness already signaled")
	// }
}

func (p *Producer) Publish(ctx context.Context, exchange, key string, body []byte) error {
	select {
	case <-p.publishOk: // Wait for readiness
	case <-ctx.Done():
		return ctx.Err()
	}

	log.Printf("Publishing message: %s", body)

	err := p.Channel.PublishWithContext(
		ctx,
		exchange,
		key,
		false,
		false,
		amqp091.Publishing{
			ContentType: "text/plain",
			Body:        body,
		},
	)
	if err != nil {
		// Re-signal readiness in case of error to not block future publishes
		select {
		case p.publishOk <- struct{}{}:
		default:
		}
		return err
	}

	return nil
}

func (p *Producer) Close() {
	close(p.exitCh)
	if p.Channel != nil {
		p.Channel.Close()
	}
	if p.Connection != nil {
		p.Connection.Conn.Close()
	}
}

func setupCloseHandler(exitCh chan struct{}) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		close(exitCh)
	}()
}
