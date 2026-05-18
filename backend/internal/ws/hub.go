// backend/internal/ws/hub.go
package ws

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	clients    map[int64]map[*websocket.Conn]*client
	broadcast  chan Message
	register   chan subscription
	unregister chan subscription
	mu         sync.RWMutex
}

type client struct {
	conn *websocket.Conn
	send chan Message
}

type Message struct {
	ExecutionID int64  `json:"execution_id"`
	Type        string `json:"type"` // "output", "complete", "error"
	Time        string `json:"time"`
	Stream      string `json:"stream,omitempty"`
	Text        string `json:"text,omitempty"`
	ExitCode    int    `json:"exit_code,omitempty"`
}

type subscription struct {
	executionID int64
	conn        *websocket.Conn
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[int64]map[*websocket.Conn]*client),
		broadcast:  make(chan Message, 256),
		register:   make(chan subscription),
		unregister: make(chan subscription),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case sub := <-h.register:
			h.mu.Lock()
			if h.clients[sub.executionID] == nil {
				h.clients[sub.executionID] = make(map[*websocket.Conn]*client)
			}
			if existing, ok := h.clients[sub.executionID][sub.conn]; ok {
				h.mu.Unlock()
				close(existing.send)
				continue
			}
			registered := &client{conn: sub.conn, send: make(chan Message, 64)}
			h.clients[sub.executionID][sub.conn] = registered
			h.mu.Unlock()
			go h.writePump(sub.executionID, registered)

		case sub := <-h.unregister:
			h.removeClient(sub.executionID, sub.conn)

		case msg := <-h.broadcast:
			h.broadcastMessage(msg)
		}
	}
}

func (h *Hub) SendOutput(executionID int64, stream, text string) {
	msg := Message{
		ExecutionID: executionID,
		Type:        "output",
		Stream:      stream,
		Text:        text,
	}
	h.broadcast <- msg
}

func (h *Hub) SendComplete(executionID int64, exitCode int) {
	msg := Message{
		ExecutionID: executionID,
		Type:        "complete",
		ExitCode:    exitCode,
	}
	h.broadcast <- msg
}

func (h *Hub) Register(execID int64, conn *websocket.Conn) {
	h.register <- subscription{executionID: execID, conn: conn}
}

func (h *Hub) Unregister(execID int64, conn *websocket.Conn) {
	h.unregister <- subscription{executionID: execID, conn: conn}
}

func (h *Hub) broadcastMessage(msg Message) {
	for _, subscriber := range h.snapshotClients(msg.ExecutionID) {
		select {
		case subscriber.send <- msg:
		default:
			go h.removeClient(msg.ExecutionID, subscriber.conn)
		}
	}
}

func (h *Hub) snapshotClients(executionID int64) []*client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := h.clients[executionID]
	if len(clients) == 0 {
		return nil
	}
	snapshot := make([]*client, 0, len(clients))
	for _, subscriber := range clients {
		snapshot = append(snapshot, subscriber)
	}
	return snapshot
}

func (h *Hub) removeClient(executionID int64, conn *websocket.Conn) {
	var subscriber *client
	h.mu.Lock()
	if clients, ok := h.clients[executionID]; ok {
		subscriber = clients[conn]
		if subscriber != nil {
			delete(clients, conn)
			if len(clients) == 0 {
				delete(h.clients, executionID)
			}
		}
	}
	h.mu.Unlock()
	if subscriber != nil {
		close(subscriber.send)
	}
}

func (h *Hub) writePump(executionID int64, subscriber *client) {
	defer subscriber.conn.Close()
	for msg := range subscriber.send {
		_ = subscriber.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := subscriber.conn.WriteJSON(msg); err != nil {
			h.removeClient(executionID, subscriber.conn)
			return
		}
	}
}
