package realtime

import (
	"io"
	"testing"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/stretchr/testify/assert"
)

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestHubRegistersAndBroadcasts(t *testing.T) {
	hub := NewHub()

	client := &Client{
		hub:  hub,
		conn: &testConn{},
		send: make(chan []byte, 1),
	}

	hub.register <- client
	waitForCondition(t, time.Second, func() bool { return hub.GetClientCount() == 1 })

	msg := []byte("hello")
	hub.Broadcast(msg)

	select {
	case got := <-client.send:
		assert.Equal(t, msg, got)
	case <-time.After(time.Second):
		t.Fatal("did not receive broadcast message")
	}

	hub.unregister <- client
	waitForCondition(t, time.Second, func() bool { return hub.GetClientCount() == 0 })
}

func TestHubBroadcastDropsSlowClient(t *testing.T) {
	hub := NewHub()
	client := &Client{
		hub:  hub,
		conn: &testConn{},
		send: make(chan []byte), // unbuffered -> backpressure
	}

	hub.register <- client
	waitForCondition(t, time.Second, func() bool { return hub.GetClientCount() == 1 })

	hub.Broadcast([]byte("msg"))

	waitForCondition(t, time.Second, func() bool { return hub.GetClientCount() == 0 })

	select {
	case _, ok := <-client.send:
		assert.False(t, ok)
	default:
		t.Fatal("client channel not closed for slow consumer")
	}
}

func TestReadPumpSignalsUnregister(t *testing.T) {
	unregister := make(chan *Client, 1)
	client := &Client{
		hub: &Hub{
			unregister: unregister,
		},
		conn: &testConn{
			readMessages: []readCall{{err: io.EOF}},
		},
		send: make(chan []byte, 1),
	}

	client.readPump()

	select {
	case got := <-unregister:
		assert.Equal(t, client, got)
	default:
		t.Fatal("client was not unregistered")
	}
}

type manualTicker struct {
	ch         chan time.Time
	stopCalled bool
}

func newManualTicker() *manualTicker {
	return &manualTicker{ch: make(chan time.Time, 1)}
}

func (t *manualTicker) C() <-chan time.Time {
	return t.ch
}

func (t *manualTicker) Stop() {
	t.stopCalled = true
}

func TestWritePumpSendsMessagesAndPings(t *testing.T) {
	manual := newManualTicker()
	originalFactory := pingTickerFactory
	pingTickerFactory = func() pingTicker { return manual }
	t.Cleanup(func() {
		pingTickerFactory = originalFactory
	})

	conn := &testConn{}
	client := &Client{
		hub:  &Hub{},
		conn: conn,
		send: make(chan []byte, 1),
	}

	done := make(chan struct{})
	go func() {
		client.writePump()
		close(done)
	}()

	// Deliver normal message
	client.send <- []byte("payload")

	waitForCondition(t, time.Second, func() bool { return conn.GetWriteMessageCount() >= 1 })
	assert.Equal(t, websocket.TextMessage, conn.GetWriteMessage(0).messageType)
	assert.Equal(t, []byte("payload"), conn.GetWriteMessage(0).payload)

	// Trigger ping via manual ticker
	manual.ch <- time.Now()
	waitForCondition(t, time.Second, func() bool { return conn.GetWriteMessageCount() >= 2 })
	assert.Equal(t, websocket.PingMessage, conn.GetWriteMessage(1).messageType)

	// Close send channel to exit
	close(client.send)
	waitForCondition(t, time.Second, func() bool { return conn.GetCloseCalls() >= 1 })

	<-done
	assert.True(t, manual.stopCalled)
}
