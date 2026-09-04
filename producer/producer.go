package producer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/mateothegreat/go-rabbitmq/connections"
	"github.com/mateothegreat/multilog"
	"github.com/rabbitmq/amqp091-go"
)

// DefaultConfirmTimeout bounds how long Publish waits for a broker
// confirmation when the caller's context carries no deadline of its own. A
// caller passing context.Background() would otherwise wait indefinitely on a
// broker that has stopped confirming without dropping the connection.
const DefaultConfirmTimeout = 30 * time.Second

var (
	// ErrNotConnected is returned by Publish before Connect has succeeded or
	// after Close.
	ErrNotConnected = errors.New("producer: not connected")

	// ErrConfirmsDisabled is returned when the channel is not in confirm mode,
	// which means a publish cannot be confirmed at all.
	ErrConfirmsDisabled = errors.New("producer: channel is not in confirm mode")

	// ErrNack is returned when the broker explicitly refused a publish.
	ErrNack = errors.New("producer: publish was nacked by the broker")

	// ErrChannelClosed is returned when the channel closed before the broker
	// confirmed a publish, which leaves the outcome of that message unknown.
	ErrChannelClosed = errors.New("producer: channel closed before confirmation")
)

// confirmation resolves when the broker settles one delivery tag. It is
// satisfied by *amqp091.DeferredConfirmation.
type confirmation interface {
	WaitContext(ctx context.Context) (bool, error)
}

// channel is the slice of *amqp091.Channel that the publish path depends on.
// It exists so the concurrency of that path can be exercised without a broker;
// amqpChannel is the only implementation used outside of tests.
type channel interface {
	publish(ctx context.Context, exchange, key string, msg amqp091.Publishing) (confirmation, error)
	isClosed() bool
	close() error
}

// amqpChannel adapts *amqp091.Channel to channel. It holds no state and makes
// no decisions, so almost nothing escapes the tests that drive channel.
type amqpChannel struct {
	ch *amqp091.Channel
}

var _ channel = amqpChannel{}
var _ confirmation = (*amqp091.DeferredConfirmation)(nil)

func (a amqpChannel) publish(ctx context.Context, exchange, key string, msg amqp091.Publishing) (confirmation, error) {
	// amqp091 allocates the delivery tag and writes the frames under the
	// channel's own mutex, so concurrent callers can neither interleave frames
	// nor race for a tag, and it tracks the returned confirmation by tag until
	// the broker settles it.
	dc, err := a.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, false, false, msg)
	if err != nil {
		return nil, err
	}
	if dc == nil {
		return nil, ErrConfirmsDisabled
	}
	return dc, nil
}

func (a amqpChannel) isClosed() bool { return a.ch.IsClosed() }

func (a amqpChannel) close() error { return a.ch.Close() }

// Producer publishes messages on a single channel in confirm mode. A Producer
// must be created by Connect and is safe for concurrent use afterwards; it
// must not be copied.
type Producer struct {
	// ConfirmTimeout bounds the wait for a broker confirmation when the
	// context passed to Publish has no deadline. Zero means
	// DefaultConfirmTimeout. Set it before the first Publish.
	ConfirmTimeout time.Duration

	// Connection and Channel expose the underlying amqp091 objects for callers
	// that need them. Connect replaces both; callers must not mutate them.
	Connection *connections.Connection
	Channel    *amqp091.Channel

	// mu guards the fields Connect and Close replace, so a publish can run
	// concurrently with either. It is only held long enough to snapshot ch,
	// never across a publish or a confirmation wait.
	mu     sync.RWMutex
	ch     channel
	closed bool

	loggers []multilog.Logger
}

func (p *Producer) WithLoggers(loggers ...multilog.Logger) *Producer {
	p.loggers = loggers
	return p
}

// Connect dials uri, opens a channel and puts it into confirm mode, retrying
// with exponential backoff for up to five minutes.
//
// Connect may be called again to replace a channel that has been lost. The
// superseded channel and connection are closed, which releases any publish
// still waiting on a confirmation from them with an error rather than leaving
// it blocked.
//
// Arguments:
//   - uri: The AMQP URI to dial.
//
// Returns:
//   - An error when no attempt within the backoff window succeeded, leaving
//     the producer unchanged.
func (p *Producer) Connect(uri string) error {
	operation := func() error {
		conn, err := connections.CreateConnection(uri)
		if err != nil {
			multilog.Trace("producer", "connect", map[string]interface{}{
				"uri":   uri,
				"error": err,
			})
			return err
		}

		ch, err := conn.Conn.Channel()
		if err != nil {
			multilog.Trace("producer", "open channel", map[string]interface{}{
				"uri":   uri,
				"error": err,
			})
			// Drop the connection so a retry does not leave one dialed per
			// failed attempt.
			conn.Conn.Close()
			return err
		}

		// Confirm mode is what makes a delivery tag, and therefore a
		// confirmation, exist for every publish.
		if err := ch.Confirm(false); err != nil {
			multilog.Trace("producer", "confirm mode", map[string]interface{}{
				"uri":   uri,
				"error": err,
				"mode":  false,
			})
			conn.Conn.Close()
			return err
		}

		p.setChannel(conn, ch)

		return nil
	}

	expBackOff := backoff.NewExponentialBackOff()
	expBackOff.MaxElapsedTime = 5 * time.Minute

	if err := backoff.Retry(operation, expBackOff); err != nil {
		multilog.Error("producer", "connect", map[string]interface{}{
			"uri":            uri,
			"maxElapsedTime": expBackOff.MaxElapsedTime,
			"error":          err,
		})
		return fmt.Errorf("producer: connect to %q: %w", uri, err)
	}

	multilog.Trace("producer", "connected", map[string]interface{}{
		"uri": uri,
	})

	return nil
}

// setChannel installs a freshly opened channel and retires the previous one.
func (p *Producer) setChannel(conn *connections.Connection, ch *amqp091.Channel) {
	oldConn := p.replaceChannel(amqpChannel{ch: ch}, conn, ch)
	if oldConn != nil && oldConn.Conn != nil && oldConn != conn {
		oldConn.Conn.Close()
	}
}

// replaceChannel swaps in next, closes the channel it supersedes and returns
// the connection that was displaced so the caller can retire it.
func (p *Producer) replaceChannel(next channel, conn *connections.Connection, ch *amqp091.Channel) *connections.Connection {
	p.mu.Lock()
	old, oldConn := p.ch, p.Connection
	p.Connection = conn
	p.Channel = ch
	p.ch = next
	p.closed = false
	p.mu.Unlock()

	// Closing outside the lock keeps Publish from stalling behind broker I/O.
	// amqp091 settles every outstanding delivery tag as unacknowledged while
	// the channel shuts down, so publishes still waiting on the old channel
	// fail instead of blocking forever.
	if old != nil {
		old.close()
	}

	return oldConn
}

// Publish sends body to exchange under the given routing key and waits for the
// broker to confirm it.
//
// Publish is safe to call from multiple goroutines and does not serialize its
// callers. Every message gets its own delivery tag and each caller waits only
// on the confirmation for its own tag, so many messages are in flight at once
// and throughput is bounded by how fast frames can be written rather than by
// one broker round trip per message.
//
// A nil return means the broker acknowledged the message. An error means it
// was refused, could not be written, or was not confirmed before the deadline;
// a deadline leaves the outcome unknown, so a caller that requires delivery
// has to retry and tolerate a duplicate.
//
// Arguments:
//   - ctx: Cancels the publish and bounds the confirmation wait.
//   - exchange: The exchange to publish to, empty for the default exchange.
//   - key: The routing key.
//   - body: The message body.
//
// Returns:
//   - nil once the broker has acknowledged the message, otherwise the reason
//     it could not be confirmed.
func (p *Producer) Publish(ctx context.Context, exchange, key string, body []byte) error {
	p.mu.RLock()
	ch, timeout := p.ch, p.ConfirmTimeout
	p.mu.RUnlock()

	if ch == nil {
		return ErrNotConnected
	}

	multilog.Debug("producer", "publish", map[string]interface{}{
		"exchange": exchange,
		"key":      key,
		"body":     string(body),
	})

	confirm, err := ch.publish(ctx, exchange, key, amqp091.Publishing{
		ContentType: "text/plain",
		Body:        body,
	})
	if err != nil {
		multilog.Trace("producer", "publish failed", map[string]interface{}{
			"exchange": exchange,
			"key":      key,
			"error":    err,
		})
		return fmt.Errorf("producer: publish to %q: %w", exchange, err)
	}

	if timeout <= 0 {
		timeout = DefaultConfirmTimeout
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	acked, err := confirm.WaitContext(waitCtx)
	if err != nil {
		multilog.Trace("producer", "confirmation not received", map[string]interface{}{
			"exchange": exchange,
			"key":      key,
			"error":    err,
		})
		return fmt.Errorf("producer: awaiting confirmation from %q: %w", exchange, err)
	}

	if !acked {
		// A shutting down channel settles its outstanding tags as
		// unacknowledged, which is indistinguishable from a broker refusal
		// except by the state of the channel itself.
		if ch.isClosed() {
			multilog.Trace("producer", "channel closed before confirmation", map[string]interface{}{
				"exchange": exchange,
				"key":      key,
			})
			return ErrChannelClosed
		}

		multilog.Trace("producer", "message nacked", map[string]interface{}{
			"exchange": exchange,
			"key":      key,
		})
		return ErrNack
	}

	multilog.Trace("producer", "message confirmed", map[string]interface{}{
		"exchange": exchange,
		"key":      key,
	})

	return nil
}

// Close releases the channel and connection. It is safe to call more than
// once, safe on a producer that never connected, and safe to call while
// publishes are in flight: closing the channel settles their delivery tags as
// unacknowledged so they return an error instead of waiting for a confirmation
// that will never arrive.
func (p *Producer) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	ch, conn := p.ch, p.Connection
	p.ch = nil
	p.mu.Unlock()

	if ch != nil {
		ch.close()
	}
	if conn != nil && conn.Conn != nil {
		conn.Conn.Close()
	}
}
