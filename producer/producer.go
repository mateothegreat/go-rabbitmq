package producer

import (
	"context"
	"log"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/mateothegreat/go-multilog/multilog"
	"github.com/mateothegreat/go-rabbitmq/connections"
	"github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	Connection   *connections.Connection
	Channel      *amqp091.Channel
	exitCh       chan struct{}
	confirms     chan amqp091.Confirmation
	confirmsDone chan struct{}
	publishOk    chan struct{}
	loggers      []multilog.Logger
}

func (p *Producer) WithLoggers(loggers ...multilog.Logger) *Producer {
	p.loggers = loggers
	return p
}

func (p *Producer) Connect(uri string) error {
	operation := func() error {
		var err error

		// Create a connection
		p.Connection, err = connections.CreateConnection(uri)
		if err != nil {
			multilog.Trace("producer", "connect", map[string]interface{}{
				"uri":   uri,
				"error": err,
			})
			return err
		}

		// Open a channel
		p.Channel, err = p.Connection.Conn.Channel()
		if err != nil {
			multilog.Trace("producer", "open channel", map[string]interface{}{
				"uri":   uri,
				"error": err,
			})
			return err
		}

		// Put the channel into confirm mode
		if err := p.Channel.Confirm(false); err != nil {
			multilog.Trace("producer", "confirm mode", map[string]interface{}{
				"uri":   uri,
				"error": err,
				"mode":  false,
			})
			return err
		}

		// Initialization succeeded, set up the rest
		p.setupChannel()
		multilog.Trace("producer", "connected", map[string]interface{}{
			"uri": uri,
		})

		return nil
	}

	// Retry with exponential backoff
	expBackOff := backoff.NewExponentialBackOff()
	expBackOff.MaxElapsedTime = 5 * time.Minute
	err := backoff.Retry(operation, expBackOff)
	multilog.Info("producer", "retrying connect", map[string]interface{}{
		"uri":            uri,
		"maxElapsedTime": expBackOff.MaxElapsedTime,
	})
	if err != nil {
		multilog.Fatal("producer", "connect", map[string]interface{}{
			"uri":   uri,
			"error": err,
		})
	}

	return nil
}

func (p *Producer) setupChannel() {
	p.exitCh = make(chan struct{})
	p.confirms = make(chan amqp091.Confirmation, 1)
	p.confirmsDone = make(chan struct{})
	p.publishOk = make(chan struct{}, 1) // Signal initial readiness

	// Start listening for confirmations.
	confirmChan := p.Channel.NotifyPublish(make(chan amqp091.Confirmation, 1))
	go p.handleConfirms(confirmChan)
	p.publishOk <- struct{}{} // Signal initial readiness
}

func (p *Producer) handleConfirms(confirmChan <-chan amqp091.Confirmation) {
	for {
		select {
		case confirm := <-confirmChan:
			if confirm.Ack {
				multilog.Trace("producer", "message confirmed", map[string]interface{}{
					"deliveryTag": confirm.DeliveryTag,
				})
			} else {
				multilog.Trace("producer", "message nack", map[string]interface{}{
					"deliveryTag": confirm.DeliveryTag,
				})
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

	multilog.Debug("producer", "publish", map[string]interface{}{
		"exchange": exchange,
		"key":      key,
		"body":     string(body),
	})

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
