// This example declares a durable exchange, and publishes one messages to that
// exchange. This example allows up to 8 outstanding publisher confirmations
// before blocking publishing.
package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nvr-ai/go-rabbitmq/connections"
	"github.com/nvr-ai/go-rabbitmq/messages"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	uri                  string
	Connection           *connections.Connection
	exitCh               chan struct{}
	confirmsCh           chan struct{}
	exiconfirmsDoneChtCh chan struct{}
	Ctx                  context.Context
}

func (p *Producer) Connect(uri string) error {
	p.uri = uri

	log.Printf("producer: dialing %s", uri)

	config := amqp.Config{
		Vhost:      "/",
		Properties: amqp.NewConnectionProperties(),
	}
	config.Properties.SetClientConnectionName("producer-with-confirms")

	connection, err := connections.CreateConnection(uri)

	if err != nil {
		return err
	}

	// defer connection.Conn.Close()

	p.Connection = connection

	return nil
}

func (p *Producer) SetupCloseHandler(exitCh chan struct{}) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Printf("close handler: Ctrl+C pressed in Terminal")
		close(p.exitCh)
	}()
}

func (p *Producer) Publish(ctx context.Context, exchange string, key string, message *messages.Message) error {
	exitCh := make(chan struct{})
	confirmsCh := make(chan *amqp.DeferredConfirmation)
	confirmsDoneCh := make(chan struct{})
	// Note: this is a buffered channel so that indicating OK to
	// publish does not block the confirm handler
	publishOkCh := make(chan struct{}, 1)

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	p.SetupCloseHandler(exitCh)
	p.StartConfirmHandler(publishOkCh, confirmsCh, confirmsDoneCh, exitCh)

	log.Println("producer: got Connection, getting Channel")

	channel, err := p.Connection.Conn.Channel()

	if err != nil {
		return fmt.Errorf("error getting a channel: %s", err)
	}

	// defer channel.Close()

	// Reliable publisher confirms require confirm.select support from the
	// connection.
	log.Printf("producer: enabling publisher confirms.")
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("producer: channel could not be put into confirm mode: %s", err)
	}

	log.Println("producer: waiting on the OK to publish...")

WaitForOkToPublish:
	for {
		select {
		case <-confirmsDoneCh:
			log.Println("producer: stopping, all confirms seen")
			break WaitForOkToPublish
		case <-publishOkCh:
			log.Println("producer: got the OK to publish")
			break WaitForOkToPublish
		case <-time.After(time.Second):
			log.Println("producer: still waiting on the OK to publish...")
			continue
		}
	}

	log.Printf("producer: %s, %s: publishing %dB body (%q)", exchange, key, len(body), body)
	dConfirmation, err := channel.PublishWithDeferredConfirmWithContext(
		ctx,
		exchange,
		key,
		true,
		false,
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "application/json",
			ContentEncoding: "",
			DeliveryMode:    amqp.Persistent,
			Priority:        0,
			AppId:           "sequential-producer",
			Body:            []byte(body),
		},
	)

	if err != nil {
		return fmt.Errorf("producer: error in publish: %s", err)
	}

	select {
	case <-confirmsDoneCh:
		log.Println("producer: stopping, all confirms seen")
		return nil
	case confirmsCh <- dConfirmation:
		log.Println("producer: delivered deferred confirm to handler")
		break
	}

	return nil
}

func (p *Producer) StartConfirmHandler(publishOkCh chan<- struct{}, confirmsCh <-chan *amqp.DeferredConfirmation, confirmsDoneCh chan struct{}, exitCh <-chan struct{}) {
	go func() {
		confirms := make(map[uint64]*amqp.DeferredConfirmation)

		// ConfirmHandlerLoop:
		for {
			// select {
			// case <-exitCh:
			// 	p.exitConfirmHandler(confirms, confirmsDoneCh)
			// 	return
			// default:
			// 	break ConfirmHandlerLoop
			// }

			outstandingConfirmationCount := len(confirms)

			// Note: 8 is arbitrary, you may wish to allow more outstanding confirms before blocking publish
			if outstandingConfirmationCount <= 8 {
				select {
				case publishOkCh <- struct{}{}:
					log.Println("confirm handler: sent OK to publish")
				case <-time.After(time.Second * 5):
					log.Println("confirm handler: timeout indicating OK to publish (this should never happen!)")
				}
			} else {
				log.Printf("confirm handler: waiting on %d outstanding confirmations, blocking publish", outstandingConfirmationCount)
			}

			select {
			case confirmation := <-confirmsCh:
				dtag := confirmation.DeliveryTag
				confirms[dtag] = confirmation
			case <-exitCh:
				p.exitConfirmHandler(confirms, confirmsDoneCh)
				return
			}

			p.checkConfirmations(confirms)
		}
	}()
}

func (p *Producer) exitConfirmHandler(confirms map[uint64]*amqp.DeferredConfirmation, confirmsDoneCh chan struct{}) {
	log.Println("confirm handler: exit requested")
	p.waitConfirmations(confirms)
	close(confirmsDoneCh)
	log.Println("confirm handler: exiting")
}

func (p *Producer) checkConfirmations(confirms map[uint64]*amqp.DeferredConfirmation) {
	log.Printf("confirm handler: checking %d outstanding confirmations", len(confirms))
	for k, v := range confirms {
		if v.Acked() {
			log.Printf("confirm handler: confirmed delivery with tag: %d", k)
			delete(confirms, k)
		}
	}
}

func (p *Producer) waitConfirmations(confirms map[uint64]*amqp.DeferredConfirmation) {
	log.Printf("confirm handler: waiting on %d outstanding confirmations", len(confirms))

	p.checkConfirmations(confirms)

	for k, v := range confirms {
		select {
		case <-v.Done():
			log.Printf("confirm handler: confirmed delivery with tag: %d", k)
			delete(confirms, k)
		case <-time.After(time.Second):
			log.Printf("confirm handler: did not receive confirmation for tag %d", k)
		}
	}

	outstandingConfirmationCount := len(confirms)
	if outstandingConfirmationCount > 0 {
		log.Printf("confirm handler: exiting with %d outstanding confirmations", outstandingConfirmationCount)
	} else {
		log.Println("confirm handler: done waiting on outstanding confirmations")
	}
}

func (p *Producer) Close() error {
	return p.Connection.Conn.Close()
}
