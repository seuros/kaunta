package realtime

import (
	"io"
	"sync"
)

type testConn struct {
	mu            sync.Mutex
	writeMessages []writeCall
	readMessages  []readCall
	closeCalls    int
}

type writeCall struct {
	messageType int
	payload     []byte
}

type readCall struct {
	messageType int
	payload     []byte
	err         error
}

func (c *testConn) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeMessages = append(c.writeMessages, writeCall{
		messageType: messageType,
		payload:     append([]byte(nil), data...),
	})
	return nil
}

func (c *testConn) ReadMessage() (messageType int, p []byte, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.readMessages) == 0 {
		return 0, nil, io.EOF
	}
	msg := c.readMessages[0]
	c.readMessages = c.readMessages[1:]
	return msg.messageType, append([]byte(nil), msg.payload...), msg.err
}

func (c *testConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeCalls++
	return nil
}

// Thread-safe getters for test assertions
func (c *testConn) GetWriteMessageCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.writeMessages)
}

func (c *testConn) GetWriteMessage(index int) writeCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeMessages[index]
}

func (c *testConn) GetCloseCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCalls
}
