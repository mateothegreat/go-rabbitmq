package producer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// The tests in this file execute the readiness-token protocol the producer used
// before it was made concurrency safe, so the diagnosis rests on observed
// behaviour rather than on reading the source. legacyProducer reproduces that
// protocol line for line from commit 0fc2a57; the only substitution is the call
// to Channel.PublishWithContext, which becomes the injectable send below so the
// protocol can run without a broker. Nothing here is used by the production
// code path; these tests exist to demonstrate the bugs that were fixed.
type legacyProducer struct {
	exitCh    chan struct{}
	publishOk chan struct{}

	// send stands in for p.Channel.PublishWithContext.
	send func(ctx context.Context, body []byte) error
}

func newLegacyProducer(send func(ctx context.Context, body []byte) error) *legacyProducer {
	p := &legacyProducer{
		exitCh:    make(chan struct{}),
		publishOk: make(chan struct{}, 1),
		send:      send,
	}
	p.publishOk <- struct{}{} // Signal initial readiness

	return p
}

// handleConfirms is the confirmation handler goroutine, verbatim apart from
// dropping the log lines.
func (p *legacyProducer) handleConfirms(confirmChan <-chan amqp091.Confirmation) {
	for {
		select {
		case <-confirmChan:
			// Re-signal readiness after each confirmation
			p.signalPublishOk()
		case <-p.exitCh:
			return
		}
	}
}

// signalPublishOk is the unguarded blocking send.
func (p *legacyProducer) signalPublishOk() {
	p.publishOk <- struct{}{}
}

func (p *legacyProducer) Publish(ctx context.Context, body []byte) error {
	select {
	case <-p.publishOk: // Wait for readiness
	case <-ctx.Done():
		return ctx.Err()
	}

	err := p.send(ctx, body)
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

// TestLegacyProtocolAllowsOneUnconfirmedPublish confirms the throughput claim:
// because the only thing that refills the readiness token is a confirmation,
// the old protocol never had more than one message awaiting the broker, no
// matter how many goroutines were publishing.
func TestLegacyProtocolAllowsOneUnconfirmedPublish(t *testing.T) {
	const (
		publishers = 32
		roundTrip  = 2 * time.Millisecond
	)

	confirmChan := make(chan amqp091.Confirmation, 1)

	var (
		mu             sync.Mutex
		unconfirmed    int
		maxUnconfirmed int
	)

	send := func(ctx context.Context, body []byte) error {
		mu.Lock()
		unconfirmed++
		if unconfirmed > maxUnconfirmed {
			maxUnconfirmed = unconfirmed
		}
		mu.Unlock()

		// Stand in for the broker: acknowledge after a round trip.
		go func() {
			time.Sleep(roundTrip)
			mu.Lock()
			unconfirmed--
			mu.Unlock()
			confirmChan <- amqp091.Confirmation{Ack: true}
		}()

		return nil
	}

	p := newLegacyProducer(send)
	go p.handleConfirms(confirmChan)
	defer close(p.exitCh)

	var wg sync.WaitGroup
	wg.Add(publishers)
	for range publishers {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := p.Publish(ctx, []byte("body")); err != nil {
				t.Errorf("legacy publish: %v", err)
			}
		}()
	}

	done := inBackground(wg.Wait)
	waitFor(t, done, 30*time.Second, "legacy publishes to drain")

	mu.Lock()
	peak := maxUnconfirmed
	mu.Unlock()

	if peak != 1 {
		t.Errorf("old protocol allowed %d unconfirmed publishes, expected it to be capped at 1", peak)
	}

	// For contrast, the same workload through the current producer.
	f := newFakeChannel()
	current := newTestProducer(f)
	gate := f.requireGate(t, publishers)
	currentDone := inBackground(func() { publishN(current, context.Background(), publishers) })
	waitFor(t, gate, 5*time.Second, "current producer to reach full concurrency")
	f.settleAll(true)
	waitFor(t, currentDone, 5*time.Second, "current publishes to return")

	_, nowInFlight, _ := f.snapshot()
	t.Logf("unconfirmed publishes in flight: old protocol %d, current producer %d", peak, nowInFlight)
}

// TestLegacyProtocolConfirmHandlerBlocksAndDoubleIssues drives the exact
// interleaving the two token bugs need. The failure path refills the token
// non-blockingly, then a confirmation for the message that was written anyway
// arrives; signalPublishOk blocks on the already full buffer, so the handler
// stops draining confirmations, and the token it is still holding admits a
// second publisher into a section meant for one.
func TestLegacyProtocolConfirmHandlerBlocksAndDoubleIssues(t *testing.T) {
	confirmChan := make(chan amqp091.Confirmation)

	failOnce := make(chan struct{}, 1)
	failOnce <- struct{}{}

	var (
		mu       sync.Mutex
		inFlight int
		peak     int
	)

	release := make(chan struct{})
	send := func(ctx context.Context, body []byte) error {
		select {
		case <-failOnce:
			// A publish that reports an error after the frames were already
			// written, so the broker still confirms it.
			return errors.New("write error after the frames went out")
		default:
		}

		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()

		// Hold inside the section so overlapping publishers are observable.
		<-release

		mu.Lock()
		inFlight--
		mu.Unlock()

		return nil
	}

	p := newLegacyProducer(send)

	handlerDone := inBackground(func() { p.handleConfirms(confirmChan) })
	releaseAll := sync.OnceFunc(func() { close(release) })
	defer func() {
		close(p.exitCh)
		releaseAll()
	}()

	// The failing publish takes the only token and hands it straight back.
	if err := p.Publish(context.Background(), []byte("fails")); err == nil {
		t.Fatal("expected the injected write error")
	}

	// The late confirmation for that message wedges the handler: the buffer is
	// full, and signalPublishOk sends unconditionally.
	confirmChan <- amqp091.Confirmation{DeliveryTag: 1, Ack: true}

	// With the handler blocked mid-send there are two tokens in circulation:
	// one buffered and one in the hands of the blocked sender.
	var wg sync.WaitGroup
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p.Publish(ctx, []byte("body"))
		}()
	}

	// Both publishers should be inside the section at once, which is precisely
	// what the token was supposed to prevent.
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		observed := peak
		mu.Unlock()
		if observed >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("only %d concurrent publishers observed, expected the token to be double-issued", observed)
		case <-time.After(time.Millisecond):
		}
	}

	// The handler is still stuck in signalPublishOk rather than back in its
	// select, so nothing is draining confirmations any more.
	select {
	case <-handlerDone:
		t.Fatal("handler returned unexpectedly")
	default:
	}

	blocked := make(chan struct{})
	go func() {
		defer close(blocked)
		select {
		case confirmChan <- amqp091.Confirmation{DeliveryTag: 2, Ack: true}:
		case <-time.After(500 * time.Millisecond):
		}
	}()
	<-blocked

	t.Log("old protocol: confirmation handler wedged on an unguarded send and the readiness token was issued twice")

	releaseAll()
	waitFor(t, inBackground(wg.Wait), 10*time.Second, "legacy publishers to finish")
}
