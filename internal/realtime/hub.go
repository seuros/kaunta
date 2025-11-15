package realtime

import (
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"

	"github.com/seuros/kaunta/internal/logging"
	"go.uber.org/zap"
)

type Hub struct {
	register    chan *Client
	unregister  chan *Client
	broadcast   chan []byte
	clientCount chan chan int // For thread-safe client count queries
	clients     map[*Client]struct{}
}

type wsConn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	Close() error
}

type Client struct {
	hub  *Hub
	conn wsConn
	send chan []byte
}

type pingTicker interface {
	C() <-chan time.Time
	Stop()
}

type realPingTicker struct {
	*time.Ticker
}

func (t *realPingTicker) C() <-chan time.Time {
	return t.Ticker.C
}

var pingTickerFactory = func() pingTicker {
	return &realPingTicker{time.NewTicker(30 * time.Second)}
}

func NewHub() *Hub {
	h := &Hub{
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan []byte, 512),
		clientCount: make(chan chan int),
		clients:     make(map[*Client]struct{}),
	}

	go h.run()
	return h
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				_ = client.conn.Close()
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		case response := <-h.clientCount:
			response <- len(h.clients)
		}
	}
}

func (h *Hub) Broadcast(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
		logging.L().Warn("dropping realtime payload", zap.String("reason", "slow consumers"))
	}
}

// GetClientCount returns the number of connected clients in a thread-safe manner
func (h *Hub) GetClientCount() int {
	response := make(chan int)
	h.clientCount <- response
	return <-response
}

func (h *Hub) Handler() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		client := &Client{
			hub:  h,
			conn: conn,
			send: make(chan []byte, 512),
		}

		h.register <- client

		go client.writePump()
		client.readPump()
	})
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
	}()

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := pingTickerFactory()
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C():
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
