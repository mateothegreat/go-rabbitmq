package producer

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nvr-ai/go-rabbitmq/connections"
	"github.com/rabbitmq/amqp091-go"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	Connection   *connections.Connection
	Channel      *amqp.Channel
	exitCh       chan struct{}
	confirms     chan *amqp091.DeferredConfirmation
	confirmsDone chan struct{}
	publishOk    chan struct{}
}

func (p *Producer) Connect(uri string) error {
	log.Printf("producer: dialing %s", uri)

	p.exitCh = make(chan struct{})
	p.confirms = make(chan *amqp.DeferredConfirmation)
	p.confirmsDone = make(chan struct{})
	// Note: this is a buffered channel so that indicating OK to
	// publish does not block the confirm handler
	p.publishOk = make(chan struct{}, 1)

	connection, err := connections.CreateConnection(uri)
	if err != nil {
		return err
	}

	p.Connection = connection

	p.Channel, err = p.Connection.Conn.Channel()
	if err != nil {
		return err
	}

	if err := p.Channel.Confirm(false); err != nil {
		log.Fatalf("producer: channel could not be put into confirm mode: %s", err)
	}

	setupCloseHandler(p.exitCh)
	startConfirmHandler(p.publishOk, p.confirms, p.confirmsDone, p.exitCh)

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
	for {
		canPublish := false
		log.Println("producer: waiting on the OK to publish...")
		for {
			select {
			case <-p.confirmsDone:
				log.Println("producer: stopping, all confirms seen")
				return nil
			case <-p.publishOk:
				log.Println("producer: got the OK to publish")
				canPublish = true
				goto endLoop
			case <-time.After(time.Second):
				log.Println("producer: still waiting on the OK to publish...")
				continue
			}
		endLoop:
			if canPublish {
				break
			}
		}

		log.Printf("producer: publishing %dB body (%q)", len(body), body)
		dConfirmation, err := p.Channel.PublishWithDeferredConfirmWithContext(
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
				Body:            []byte(body),
			},
		)
		if err != nil {
			log.Fatalf("producer: error in publish: %s", err)
		}

		select {
		case <-p.confirmsDone:
			log.Println("producer: stopping, all confirms seen")
			return nil
		case p.confirms <- dConfirmation:
			log.Println("producer: delivered deferred confirm to handler")
			break
		}

		select {
		case <-p.confirmsDone:
			log.Println("producer: stopping, all confirms seen")
		case <-time.After(time.Millisecond * 250):
			log.Println("producer: initiating stop")
			close(p.exitCh)
			select {
			case <-p.confirmsDone:
				log.Println("producer: stopping, all confirms seen")
				return nil
			case <-time.After(time.Second * 10):
				log.Println("producer: may be stopping with outstanding confirmations")
				return nil
			}
		}
	}
}

// }

func startConfirmHandler(publishOkCh chan<- struct{}, confirmsCh <-chan *amqp.DeferredConfirmation, confirmsDoneCh chan struct{}, exitCh <-chan struct{}) {
	go func() {
		confirms := make(map[uint64]*amqp.DeferredConfirmation)

		for {
			select {
			case <-exitCh:
				exitConfirmHandler(confirms, confirmsDoneCh)
				return
			default:
				break
			}

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
				exitConfirmHandler(confirms, confirmsDoneCh)
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
