package producer

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nvr-ai/go-rabbitmq/connections"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	Connection     *connections.Connection
	Channel        *amqp.Channel
	publishOkCh    chan struct{}
	confirmsCh     chan *amqp.DeferredConfirmation
	confirmsDoneCh chan struct{}
	exitCh         chan struct{}
}

func (p *Producer) Connect(uri string) error {
	log.Printf("producer: dialing %s", uri)

	connection, err := connections.CreateConnection(uri)

	if err != nil {
		println(err.Error())
		return err
	}

	p.Connection = connection

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

func (p *Producer) Publish(ctx context.Context, exchange string, key string, body []byte) error {
	p.exitCh = make(chan struct{})
	p.publishOkCh = make(chan struct{})
	p.confirmsCh = make(chan *amqp.DeferredConfirmation)
	p.confirmsDoneCh = make(chan struct{})
	p.startConfirmHandler()

	config := amqp.Config{
		Vhost:      "/",
		Properties: amqp.NewConnectionProperties(),
	}
	config.Properties.SetClientConnectionName("producer-with-confirms")

	// Reliable publisher confirms require confirm.select support from the
	// connection.
	log.Printf("producer: enabling publisher confirms.")
	if err := p.Connection.Channel.Confirm(false); err != nil {
		log.Fatalf("producer: channel could not be put into confirm mode: %s", err)
	}

	for {
		canPublish := false
		log.Println("producer: waiting on the OK to publish...")
		for {
			select {
			case <-p.confirmsDoneCh:
				log.Println("producer: stopping, all confirms seen")
				return nil
			case <-p.publishOkCh:
				log.Println("producer: got the OK to publish")
				canPublish = true
				break
			case <-time.After(time.Second):
				log.Println("producer: still waiting on the OK to publish...")
				continue
			}
			if canPublish {
				break
			}
		}

		log.Printf("producer: publishing %dB body (%q)", len(body), body)

		dConfirmation, err := p.Connection.Channel.PublishWithDeferredConfirmWithContext(
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
			log.Fatalf("producer: error in publish: %s", err)
		}

		select {
		case <-p.confirmsDoneCh:
			log.Println("producer: stopping, all confirms seen")
			return nil
		case p.confirmsCh <- dConfirmation:
			log.Println("producer: delivered deferred confirm to handler")
			break
		}

		select {
		case <-p.confirmsDoneCh:
			log.Println("producer: stopping, all confirms seen")
			return nil
		default:
			log.Println("producer: initiating stop")
			close(p.exitCh)
			select {
			case <-p.confirmsDoneCh:
				log.Println("producer: stopping, all confirms seen")
				return nil
			case <-time.After(time.Second * 10):
				log.Println("producer: may be stopping with outstanding confirmations")
				return nil
			}
		}
	}
}

func (p *Producer) startConfirmHandler() {
	go func() {
		confirms := make(map[uint64]*amqp.DeferredConfirmation)

		for {
			select {
			case <-p.exitCh:
				exitConfirmHandler(confirms, p.confirmsDoneCh)
				return
			default:
				break
			}

			outstandingConfirmationCount := len(confirms)

			// Note: 8 is arbitrary, you may wish to allow more outstanding confirms before blocking publish
			if outstandingConfirmationCount == 0 {
				select {
				case p.publishOkCh <- struct{}{}:
					log.Println("confirm handler: sent OK to publish")
				case <-time.After(time.Second * 5):
					log.Println("confirm handler: timeout indicating OK to publish (this should never happen!)")
				}
			} else {
				log.Printf("confirm handler: waiting on %d outstanding confirmations, blocking publish", outstandingConfirmationCount)
			}

			select {
			case confirmation := <-p.confirmsCh:
				dtag := confirmation.DeliveryTag
				confirms[dtag] = confirmation
			case <-p.exitCh:
				exitConfirmHandler(confirms, p.confirmsDoneCh)
				return
			}

			checkConfirmations(confirms)
		}
	}()
}

func exitConfirmHandler(confirms map[uint64]*amqp.DeferredConfirmation, confirmsDoneCh chan struct{}) {
	log.Println("confirm handler: exit requested")
	waitConfirmations(confirms)
	close(confirmsDoneCh)
	log.Println("confirm handler: exiting")
}

func checkConfirmations(confirms map[uint64]*amqp.DeferredConfirmation) {
	log.Printf("confirm handler: checking %d outstanding confirmations", len(confirms))
	for k, v := range confirms {
		if v.Acked() {
			log.Printf("confirm handler: confirmed delivery with tag: %d", k)
			delete(confirms, k)
		}
	}
}

func waitConfirmations(confirms map[uint64]*amqp.DeferredConfirmation) {
	log.Printf("confirm handler: waiting on %d outstanding confirmations", len(confirms))

	checkConfirmations(confirms)

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
