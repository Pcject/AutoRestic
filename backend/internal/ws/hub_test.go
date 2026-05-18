package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHubBroadcastsMessagesToRegisteredClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		hub.Register(99, conn)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	hub.SendOutput(99, "stdout", "hello")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var msg Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "output" || msg.Text != "hello" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestHubDropsBackpressuredClientQueues(t *testing.T) {
	hub := NewHub()
	conn := &websocket.Conn{}
	subscriber := &client{conn: conn, send: make(chan Message, 1)}
	hub.clients[7] = map[*websocket.Conn]*client{conn: subscriber}
	subscriber.send <- Message{ExecutionID: 7, Type: "output", Text: "occupied"}

	hub.broadcastMessage(Message{ExecutionID: 7, Type: "output", Text: "next"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		_, ok := hub.clients[7][conn]
		hub.mu.RUnlock()
		if !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected backpressured client to be removed")
}
