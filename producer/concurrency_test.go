package producer

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// These tests drive Publish through the channel seam rather than a broker, so
// they exercise the producer's own synchronization: delivery tag handling,
// concurrent confirmation waits, the publish failure path, and what happens to
// in-flight publishes when the channel is replaced or closed. What they cannot
// cover is amqp091 and broker behaviour; that is what producer_test.go does
// when a broker is reachable.

// fakeChannel stands in for an amqp091 channel. Every publish is recorded and
// left unsettled until a test decides how its delivery tag resolves, which is
// what makes the confirmation timing controllable.
type fakeChannel struct {
	mu      sync.Mutex
	tag     uint64
	pending map[uint64]*fakeConfirmation
	bodies  []string
	closed  bool

	// publishErr, when set, fails every publish the way a write error on a
	// real channel would.
	publishErr error

	inFlight    int
	maxInFlight int

	// gate is closed once minInFlight publishes are outstanding at the same
	// time, which lets a test insist on real parallelism instead of merely
	// safe serialization.
	gate        chan struct{}
	minInFlight int
}

func newFakeChannel() *fakeChannel {
	return &fakeChannel{pending: make(map[uint64]*fakeConfirmation)}
}

// fakeConfirmation mirrors *amqp091.DeferredConfirmation: ack is written before
// done is closed, so a receive on done orders the read of ack.
type fakeConfirmation struct {
	done chan struct{}
	ack  bool
}

func (c *fakeConfirmation) WaitContext(ctx context.Context) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-c.done:
		return c.ack, nil
	}
}

func (c *fakeConfirmation) resolve(ack bool) {
	c.ack = ack
	close(c.done)
}

func (f *fakeChannel) publish(ctx context.Context, exchange, key string, msg amqp091.Publishing) (confirmation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.publishErr != nil {
		return nil, f.publishErr
	}
	if f.closed {
		return nil, errors.New("fake: channel is closed")
	}

	f.tag++
	c := &fakeConfirmation{done: make(chan struct{})}
	f.pending[f.tag] = c
	f.bodies = append(f.bodies, string(msg.Body))

	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	if f.gate != nil && f.inFlight >= f.minInFlight {
		close(f.gate)
		f.gate = nil
	}

	return c, nil
}

func (f *fakeChannel) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closed
}

// close settles every outstanding tag as unacknowledged, which is what amqp091
// does to its deferred confirmations while a channel shuts down.
func (f *fakeChannel) close() error {
	f.mu.Lock()
	f.closed = true
	settled := f.settleLocked(f.tag)
	f.mu.Unlock()

	resolveAll(settled, false)

	return nil
}

// settleLocked removes every pending tag up to and including tag and returns
// the confirmations to resolve, the way a broker ack carrying the multiple flag
// settles a whole range at once.
func (f *fakeChannel) settleLocked(tag uint64) []*fakeConfirmation {
	var settled []*fakeConfirmation
	for t, c := range f.pending {
		if t <= tag {
			settled = append(settled, c)
			delete(f.pending, t)
		}
	}
	f.inFlight -= len(settled)

	return settled
}

// settleThrough settles every tag up to and including tag and reports how many
// were resolved.
func (f *fakeChannel) settleThrough(tag uint64, ack bool) int {
	f.mu.Lock()
	settled := f.settleLocked(tag)
	f.mu.Unlock()

	resolveAll(settled, ack)

	return len(settled)
}

// settleAll settles everything currently outstanding.
func (f *fakeChannel) settleAll(ack bool) int {
	f.mu.Lock()
	tag := f.tag
	f.mu.Unlock()

	return f.settleThrough(tag, ack)
}

func (f *fakeChannel) setPublishErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.publishErr = err
}

func (f *fakeChannel) snapshot() (bodies []string, maxInFlight, pending int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.bodies...), f.maxInFlight, len(f.pending)
}

func (f *fakeChannel) requireGate(t *testing.T, n int) chan struct{} {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	f.gate = make(chan struct{})
	f.minInFlight = n

	return f.gate
}

func resolveAll(settled []*fakeConfirmation, ack bool) {
	for _, c := range settled {
		c.resolve(ack)
	}
}

// autoSettle keeps settling outstanding tags until stop is closed, standing in
// for a broker that streams confirmations back while publishes keep arriving.
func (f *fakeChannel) autoSettle(stop <-chan struct{}, ack bool) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				f.settleAll(ack)
				return
			default:
				f.settleAll(ack)
				time.Sleep(50 * time.Microsecond)
			}
		}
	}()

	return &wg
}

// newTestProducer returns a producer publishing through f instead of a broker.
func newTestProducer(f channel) *Producer {
	p := &Producer{ConfirmTimeout: 10 * time.Second}
	p.replaceChannel(f, nil, nil)

	return p
}

// publishN runs n concurrent publishes and returns their errors indexed by
// goroutine, so a caller can assert on every outcome.
func publishN(p *Producer, ctx context.Context, n int) []error {
	errs := make([]error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			errs[i] = p.Publish(ctx, "test-exchange", "test-key", []byte(fmt.Sprintf("message-%d", i)))
		}()
	}
	wg.Wait()

	return errs
}

// waitFor fails the test if done has not fired within d, which is how these
// tests distinguish "released with an error" from "blocked forever".
func waitFor(t *testing.T, done <-chan struct{}, d time.Duration, what string) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("timed out after %s waiting for %s", d, what)
	}
}

func inBackground(fn func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	return done
}

// TestPublishConcurrentlyAllConfirmed is the headline case: many goroutines
// publishing at once while confirmations stream back, with no external
// locking. Every message must be written exactly once and every caller must
// see its own confirmation.
func TestPublishConcurrentlyAllConfirmed(t *testing.T) {
	const n = 500

	f := newFakeChannel()
	p := newTestProducer(f)

	stop := make(chan struct{})
	settler := f.autoSettle(stop, true)

	errs := publishN(p, context.Background(), n)

	close(stop)
	settler.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("publish %d: unexpected error: %v", i, err)
		}
	}

	bodies, maxInFlight, pending := f.snapshot()
	if len(bodies) != n {
		t.Errorf("wrote %d messages, want %d", len(bodies), n)
	}
	if pending != 0 {
		t.Errorf("%d delivery tags left outstanding, want 0", pending)
	}

	seen := make(map[string]int, len(bodies))
	for _, b := range bodies {
		seen[b]++
	}
	for i := range n {
		body := fmt.Sprintf("message-%d", i)
		if seen[body] != 1 {
			t.Errorf("body %q written %d times, want exactly 1", body, seen[body])
		}
	}

	t.Logf("peak concurrent publishes in flight: %d", maxInFlight)
}

// TestPublishRunsInParallel proves the fix delivers real concurrency rather
// than safe serialization. The fake withholds every confirmation until it has
// seen 16 publishes outstanding simultaneously, so a producer that allowed
// only one in-flight publish per round trip would deadlock here.
func TestPublishRunsInParallel(t *testing.T) {
	const n = 16

	f := newFakeChannel()
	gate := f.requireGate(t, n)
	p := newTestProducer(f)

	var errs []error
	done := inBackground(func() { errs = publishN(p, context.Background(), n) })

	waitFor(t, gate, 5*time.Second, fmt.Sprintf("%d publishes to be in flight at once", n))

	// Only now, with all of them waiting on their own delivery tag, let the
	// confirmations through.
	f.settleAll(true)
	waitFor(t, done, 5*time.Second, "publishes to return")

	for i, err := range errs {
		if err != nil {
			t.Errorf("publish %d: unexpected error: %v", i, err)
		}
	}

	if _, maxInFlight, _ := f.snapshot(); maxInFlight < n {
		t.Errorf("peak in-flight publishes was %d, want at least %d", maxInFlight, n)
	}
}

// TestPublishSettlesRangeOnMultipleAck covers a broker ack with the multiple
// flag set, which confirms every tag up to and including the one it names.
func TestPublishSettlesRangeOnMultipleAck(t *testing.T) {
	const n = 64

	f := newFakeChannel()
	gate := f.requireGate(t, n)
	p := newTestProducer(f)

	var errs []error
	done := inBackground(func() { errs = publishN(p, context.Background(), n) })

	waitFor(t, gate, 5*time.Second, "all publishes to be in flight")

	// One settlement naming the highest tag has to release every waiter.
	if settled := f.settleThrough(uint64(n), true); settled != n {
		t.Errorf("multiple ack settled %d tags, want %d", settled, n)
	}
	waitFor(t, done, 5*time.Second, "publishes to return")

	for i, err := range errs {
		if err != nil {
			t.Errorf("publish %d: unexpected error: %v", i, err)
		}
	}
}

// TestPublishReportsNack checks a broker refusal reaches the caller instead of
// being swallowed into a log line.
func TestPublishReportsNack(t *testing.T) {
	const n = 32

	f := newFakeChannel()
	gate := f.requireGate(t, n)
	p := newTestProducer(f)

	var errs []error
	done := inBackground(func() { errs = publishN(p, context.Background(), n) })

	waitFor(t, gate, 5*time.Second, "all publishes to be in flight")
	f.settleAll(false)
	waitFor(t, done, 5*time.Second, "publishes to return")

	for i, err := range errs {
		if !errors.Is(err, ErrNack) {
			t.Errorf("publish %d: got %v, want ErrNack", i, err)
		}
	}
}

// TestPublishErrorPathDoesNotWedge is the regression test for the failure path.
// The old producer handed a readiness token back non-blockingly when a publish
// failed, which could double-issue it. Failing every publish concurrently and
// then succeeding proves no state survives a failure.
func TestPublishErrorPathDoesNotWedge(t *testing.T) {
	const n = 64

	wantErr := errors.New("write to a closed socket")

	f := newFakeChannel()
	f.setPublishErr(wantErr)
	p := newTestProducer(f)

	errs := publishN(p, context.Background(), n)
	for i, err := range errs {
		if !errors.Is(err, wantErr) {
			t.Errorf("publish %d: got %v, want %v", i, err, wantErr)
		}
	}

	if _, _, pending := f.snapshot(); pending != 0 {
		t.Errorf("%d delivery tags outstanding after failures, want 0", pending)
	}

	// The producer has to be fully usable again afterwards.
	f.setPublishErr(nil)

	stop := make(chan struct{})
	settler := f.autoSettle(stop, true)
	errs = publishN(p, context.Background(), n)
	close(stop)
	settler.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("publish %d after recovery: unexpected error: %v", i, err)
		}
	}
}

// TestPublishInterleavedFailuresAndSuccesses hammers both paths at once, with
// the failure mode flipping underneath the publishers, which is the shape that
// used to corrupt the token count.
func TestPublishInterleavedFailuresAndSuccesses(t *testing.T) {
	const (
		publishers = 32
		rounds     = 50
	)

	f := newFakeChannel()
	p := newTestProducer(f)

	stop := make(chan struct{})
	settler := f.autoSettle(stop, true)

	flipper := inBackground(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if i%2 == 0 {
				f.setPublishErr(errors.New("transient write failure"))
			} else {
				f.setPublishErr(nil)
			}
			time.Sleep(100 * time.Microsecond)
		}
	})

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		confirmed int
		failed    int
	)
	wg.Add(publishers)
	for range publishers {
		go func() {
			defer wg.Done()
			for r := range rounds {
				err := p.Publish(context.Background(), "x", "k", []byte(fmt.Sprintf("m-%d", r)))
				mu.Lock()
				if err == nil {
					confirmed++
				} else {
					failed++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	close(stop)
	settler.Wait()
	waitFor(t, flipper, 5*time.Second, "failure injector to stop")

	if total := confirmed + failed; total != publishers*rounds {
		t.Errorf("accounted for %d publishes, want %d", total, publishers*rounds)
	}
	if confirmed == 0 || failed == 0 {
		t.Fatalf("wanted both outcomes exercised, got %d confirmed and %d failed", confirmed, failed)
	}
	if _, _, pending := f.snapshot(); pending != 0 {
		t.Errorf("%d delivery tags outstanding at the end, want 0", pending)
	}

	t.Logf("%d confirmed, %d failed", confirmed, failed)
}

// TestChannelCloseReleasesInFlightPublishes is the "no waiter blocks forever"
// case: publishes are outstanding when the channel dies, and every one of them
// has to come back with an error.
func TestChannelCloseReleasesInFlightPublishes(t *testing.T) {
	const n = 32

	f := newFakeChannel()
	gate := f.requireGate(t, n)
	p := newTestProducer(f)

	var errs []error
	done := inBackground(func() { errs = publishN(p, context.Background(), n) })

	waitFor(t, gate, 5*time.Second, "all publishes to be in flight")
	f.close()
	waitFor(t, done, 5*time.Second, "publishes to be released by the channel closing")

	for i, err := range errs {
		if !errors.Is(err, ErrChannelClosed) {
			t.Errorf("publish %d: got %v, want ErrChannelClosed", i, err)
		}
	}
}

// TestReconnectReleasesInFlightPublishes covers Connect replacing a lost
// channel while publishes are still waiting on the old one. The old waiters
// must be released and the producer must keep working on the new channel.
func TestReconnectReleasesInFlightPublishes(t *testing.T) {
	const n = 16

	first := newFakeChannel()
	gate := first.requireGate(t, n)
	p := newTestProducer(first)

	var errs []error
	done := inBackground(func() { errs = publishN(p, context.Background(), n) })

	waitFor(t, gate, 5*time.Second, "all publishes to be in flight")

	// This is the swap Connect performs when it reopens a channel.
	second := newFakeChannel()
	p.replaceChannel(second, nil, nil)

	waitFor(t, done, 5*time.Second, "publishes on the retired channel to be released")
	for i, err := range errs {
		if !errors.Is(err, ErrChannelClosed) {
			t.Errorf("publish %d: got %v, want ErrChannelClosed", i, err)
		}
	}

	stop := make(chan struct{})
	settler := second.autoSettle(stop, true)
	errs = publishN(p, context.Background(), n)
	close(stop)
	settler.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("publish %d on the new channel: unexpected error: %v", i, err)
		}
	}
	if bodies, _, _ := second.snapshot(); len(bodies) != n {
		t.Errorf("new channel wrote %d messages, want %d", len(bodies), n)
	}
}

// TestCloseWhilePublishesInFlight closes the producer under load and checks
// Close is idempotent and safe on a producer that never connected, which the
// previous implementation was not.
func TestCloseWhilePublishesInFlight(t *testing.T) {
	const n = 32

	f := newFakeChannel()
	gate := f.requireGate(t, n)
	p := newTestProducer(f)

	var errs []error
	done := inBackground(func() { errs = publishN(p, context.Background(), n) })

	waitFor(t, gate, 5*time.Second, "all publishes to be in flight")

	p.Close()
	waitFor(t, done, 5*time.Second, "publishes to be released by Close")

	for i, err := range errs {
		if err == nil {
			t.Errorf("publish %d: got a confirmation from a closed producer", i)
		}
	}

	// Closing twice, and closing a producer that never connected, must not
	// panic; the old implementation closed an unguarded channel in both cases.
	p.Close()
	(&Producer{}).Close()

	if err := p.Publish(context.Background(), "x", "k", []byte("after close")); !errors.Is(err, ErrNotConnected) {
		t.Errorf("publish after Close: got %v, want ErrNotConnected", err)
	}
}

// TestPublishWithoutConnectFailsFast checks an unconnected producer reports the
// problem instead of blocking on a nil readiness channel until its context
// expires, which is what the old code did to a caller passing
// context.Background().
func TestPublishWithoutConnectFailsFast(t *testing.T) {
	p := &Producer{}

	done := inBackground(func() {
		if err := p.Publish(context.Background(), "x", "k", []byte("body")); !errors.Is(err, ErrNotConnected) {
			t.Errorf("got %v, want ErrNotConnected", err)
		}
	})

	waitFor(t, done, 2*time.Second, "publish on an unconnected producer to return")
}

// TestPublishConfirmTimeoutBoundsBackgroundContext checks ConfirmTimeout keeps
// a caller with no deadline of its own from waiting forever on a broker that
// has gone quiet. Every call site in go-rest passes context.Background().
func TestPublishConfirmTimeoutBoundsBackgroundContext(t *testing.T) {
	f := newFakeChannel()
	p := newTestProducer(f)
	p.ConfirmTimeout = 100 * time.Millisecond

	var err error
	done := inBackground(func() {
		err = p.Publish(context.Background(), "x", "k", []byte("never confirmed"))
	})

	waitFor(t, done, 5*time.Second, "publish to give up on the confirmation")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want a deadline exceeded error", err)
	}
}

// TestPublishHonoursCallerDeadline checks the caller's own deadline still wins.
func TestPublishHonoursCallerDeadline(t *testing.T) {
	f := newFakeChannel()
	p := newTestProducer(f)
	p.ConfirmTimeout = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var err error
	done := inBackground(func() { err = p.Publish(ctx, "x", "k", []byte("never confirmed")) })

	waitFor(t, done, 5*time.Second, "publish to observe the caller's deadline")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want a deadline exceeded error", err)
	}
}

// TestPublishLeaksNoGoroutines guards against the leak the old producer had:
// every reconnect started a confirmation handler and abandoned the previous
// one, blocked forever on a readiness send.
func TestPublishLeaksNoGoroutines(t *testing.T) {
	const (
		reconnects = 8
		perRound   = 32
	)

	settle := func(p *Producer, f *fakeChannel) {
		stop := make(chan struct{})
		settler := f.autoSettle(stop, true)
		publishN(p, context.Background(), perRound)
		close(stop)
		settler.Wait()
	}

	f := newFakeChannel()
	p := newTestProducer(f)
	settle(p, f)

	before := runtime.NumGoroutine()

	for range reconnects {
		next := newFakeChannel()
		p.replaceChannel(next, nil, nil)
		settle(p, next)
	}
	p.Close()

	// Let anything that is genuinely finishing wind down before counting.
	var after int
	for range 50 {
		time.Sleep(20 * time.Millisecond)
		if after = runtime.NumGoroutine(); after <= before {
			break
		}
	}

	if after > before {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines grew from %d to %d across %d reconnects:\n%s",
			before, after, reconnects, buf[:runtime.Stack(buf, true)])
	}
}
