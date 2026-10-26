package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var clientWsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return isAllowedOrigin(origin, r.Host)
	},
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// ClientConn encapsulates a connected browser/client WebSocket session
type ClientConn struct {
	hub  *ClientHub
	ws   *websocket.Conn
	send chan []byte
	done chan struct{}
	mu   sync.Mutex
}

// ClientHub manages all active frontend WebSocket connections and broadcasts
type ClientHub struct {
	clients    map[*ClientConn]bool
	register   chan *ClientConn
	unregister chan *ClientConn
	broadcast  chan []byte
	stopCh     chan struct{}
	running    bool
	mu         sync.RWMutex
}

// NewClientHub creates a new frontend WebSocket hub
func NewClientHub() *ClientHub {
	return &ClientHub{
		clients:    make(map[*ClientConn]bool),
		register:   make(chan *ClientConn, 64),
		unregister: make(chan *ClientConn, 64),
		broadcast:  make(chan []byte, 256),
		stopCh:     make(chan struct{}),
	}
}

// Start begins the hub dispatch loop in a background goroutine
func (h *ClientHub) Start() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	go h.run()
}

func (h *ClientHub) run() {
	for {
		select {
		case <-h.stopCh:
			h.mu.Lock()
			for client := range h.clients {
				client.close()
				delete(h.clients, client)
			}
			h.running = false
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.close()
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Slow or unresponsive client: unregister and close
					go func(c *ClientConn) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Stop gracefully shuts down the hub and disconnects all clients
func (h *ClientHub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	close(h.stopCh)
}

// Broadcast sends a raw byte slice to all connected clients
func (h *ClientHub) Broadcast(msg []byte) {
	h.mu.RLock()
	running := h.running
	h.mu.RUnlock()
	if !running {
		return
	}

	select {
	case h.broadcast <- msg:
	default:
	}
}

// BroadcastJSON serializes v to JSON and broadcasts to all clients
func (h *ClientHub) BroadcastJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.Broadcast(data)
	return nil
}

// ClientCount returns the number of currently active client connections
func (h *ClientHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (c *ClientConn) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
	default:
		close(c.done)
		_ = c.ws.Close()
	}
}

// handleClientWebSocket upgrades the HTTP connection to a WebSocket for frontend instances
func (s *Server) handleClientWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.wsHub == nil {
		http.Error(w, "WebSocket hub unavailable", http.StatusServiceUnavailable)
		return
	}

	ws, err := clientWsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := &ClientConn{
		hub:  s.wsHub,
		ws:   ws,
		send: make(chan []byte, 64),
		done: make(chan struct{}),
	}

	s.wsHub.register <- conn

	// Send initial greeting with timestamp
	initialMsg, _ := json.Marshal(map[string]any{
		"type":      "connected",
		"timestamp": time.Now().UnixMilli(),
	})
	_ = ws.WriteMessage(websocket.TextMessage, initialMsg)

	go conn.writePump()
	go conn.readPump()
}

func (c *ClientConn) readPump() {
	defer func() {
		c.hub.unregister <- c
	}()

	c.ws.SetReadLimit(64 * 1024)
	_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.ws.SetPongHandler(func(string) error {
		_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		msgType, msg, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		// If client sends ping message, respond with pong
		if msgType == websocket.TextMessage {
			var m struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &m); err == nil && m.Type == "ping" {
				pongMsg, _ := json.Marshal(map[string]any{
					"type":      "pong",
					"timestamp": time.Now().UnixMilli(),
				})
				c.mu.Lock()
				_ = c.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
				_ = c.ws.WriteMessage(websocket.TextMessage, pongMsg)
				c.mu.Unlock()
			}
		}
	}
}

func (c *ClientConn) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		c.hub.unregister <- c
	}()

	for {
		select {
		case <-c.done:
			return

		case message, ok := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.ws.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// broadcastClusterChange sends a cluster_updated notification to all connected frontend clients
func (s *Server) broadcastClusterChange(event string) {
	if s.wsHub != nil {
		_ = s.wsHub.BroadcastJSON(map[string]any{
			"type":      "cluster_updated",
			"event":     event,
			"timestamp": time.Now().UnixMilli(),
		})
	}
}
