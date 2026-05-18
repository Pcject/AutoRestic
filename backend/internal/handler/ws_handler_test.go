package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/autorestic/autorestic/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestWsHandlerIssuesBoundShortLivedTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewWsHandler(ws.NewHub(), "secret")
	h.ticketTTL = time.Minute

	router := gin.New()
	api := router.Group("/api/v1")
	h.RegisterAPI(api)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ws/executions/42/ticket", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Ticket == "" {
		t.Fatal("expected ticket to be returned")
	}
	if !h.validateTicket(body.Ticket, 42, time.Now()) {
		t.Fatal("expected issued ticket to validate for matching execution id")
	}
	if h.validateTicket(body.Ticket, 43, time.Now()) {
		t.Fatal("expected ticket to be bound to one execution id")
	}
	if h.validateTicket(body.Ticket, 42, time.Now().Add(2*time.Minute)) {
		t.Fatal("expected expired ticket to be rejected")
	}
}

func TestWsHandlerRejectsLongLivedTokenQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewWsHandler(ws.NewHub(), "secret")
	router := gin.New()
	h.Register(router)

	req := httptest.NewRequest(http.MethodGet, "/ws/executions/42?token=secret", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected long-lived query token to be rejected, got %d", resp.Code)
	}
	if strings.Contains(resp.Body.String(), "secret") {
		t.Fatal("response should not echo token")
	}
}

func TestWsHandlerAcceptsTicketWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)

	hub := ws.NewHub()
	go hub.Run()

	h := NewWsHandler(hub, "secret")
	ticket, _, err := h.issueTicket(42, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	h.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http", "ws", 1) + "/ws/executions/42?ticket=" + ticket
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("expected websocket ticket to connect, status=%d err=%v", resp.StatusCode, err)
		}
		t.Fatalf("expected websocket ticket to connect: %v", err)
	}
	defer conn.Close()
}
