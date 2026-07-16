package api

import (
	"sync"

	"github.com/gorilla/websocket"
)

// hub fans out live observation events to all connected WebSocket clients.
type hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]chan []byte
}

func newHub() *hub {
	return &hub{clients: make(map[*websocket.Conn]chan []byte)}
}

// add registers a connection and returns its outbound buffer channel.
func (h *hub) add(c *websocket.Conn) chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.clients[c] = ch
	h.mu.Unlock()
	return ch
}

func (h *hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	if ch, ok := h.clients[c]; ok {
		close(ch)
		delete(h.clients, c)
	}
	h.mu.Unlock()
}

// broadcast sends a message to every connected client, dropping it for any
// client whose buffer is full rather than blocking ingest.
func (h *hub) broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.clients {
		select {
		case ch <- msg:
		default: // slow client — drop this frame
		}
	}
}
