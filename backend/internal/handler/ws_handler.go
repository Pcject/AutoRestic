// backend/internal/handler/ws_handler.go
package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/autorestic/autorestic/internal/ws"
	"github.com/gin-gonic/gin"
)

const wsTicketTTL = time.Minute

type WsHandler struct {
	hub          *ws.Hub
	authToken    string
	allowOrigins map[string]bool
	ticketSecret []byte
	ticketTTL    time.Duration
}

type wsTicketPayload struct {
	ExecutionID int64  `json:"execution_id"`
	ExpiresAt   int64  `json:"exp"`
	Nonce       string `json:"nonce"`
}

func NewWsHandler(hub *ws.Hub, authToken string, corsOrigins ...string) *WsHandler {
	origins := map[string]bool{}
	if len(corsOrigins) > 0 {
		for _, part := range strings.Split(corsOrigins[0], ",") {
			origin := strings.TrimSpace(part)
			if origin != "" {
				origins[origin] = true
			}
		}
	}
	return &WsHandler{
		hub:          hub,
		authToken:    authToken,
		allowOrigins: origins,
		ticketSecret: deriveWsTicketSecret(authToken),
		ticketTTL:    wsTicketTTL,
	}
}

func (h *WsHandler) RegisterAPI(rg *gin.RouterGroup) {
	rg.POST("/ws/executions/:id/ticket", h.IssueTicket)
}

func (h *WsHandler) Register(r *gin.Engine) {
	r.GET("/ws/executions/:id", h.handleWS)
}

func (h *WsHandler) IssueTicket(c *gin.Context) {
	execID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
		return
	}

	ticket, expiresAt, err := h.issueTicket(execID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue websocket ticket"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ticket":     ticket,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
		"expires_in": int(h.ticketTTL.Seconds()),
	})
}

func (h *WsHandler) handleWS(c *gin.Context) {
	if !h.originAllowed(c.GetHeader("Origin")) {
		c.JSON(http.StatusForbidden, gin.H{"error": "origin not allowed"})
		return
	}

	execID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution id"})
		return
	}
	if !h.validateTicket(c.Query("ticket"), execID, time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conn, err := ws.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: upgrade failed: %v", err)
		return
	}

	h.hub.Register(execID, conn)
	defer h.hub.Unregister(execID, conn)

	// Keep connection alive, read messages (for now just hold connection)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *WsHandler) originAllowed(origin string) bool {
	if origin == "" || h.authToken == "" || len(h.allowOrigins) == 0 {
		return true
	}
	return h.allowOrigins[origin]
}

func (h *WsHandler) issueTicket(execID int64, now time.Time) (string, time.Time, error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", time.Time{}, err
	}
	expiresAt := now.Add(h.ticketTTL)
	payload := wsTicketPayload{
		ExecutionID: execID,
		ExpiresAt:   expiresAt.Unix(),
		Nonce:       base64.RawURLEncoding.EncodeToString(nonceBytes),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return encodedPayload + "." + h.signTicketPayload(encodedPayload), expiresAt, nil
}

func (h *WsHandler) validateTicket(ticket string, execID int64, now time.Time) bool {
	parts := strings.Split(ticket, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(h.signTicketPayload(parts[0]))
	if err != nil || !hmac.Equal(signature, expected) {
		return false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var payload wsTicketPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return false
	}
	return payload.ExecutionID == execID && payload.ExpiresAt > now.Unix() && payload.Nonce != ""
}

func (h *WsHandler) signTicketPayload(encodedPayload string) string {
	mac := hmac.New(sha256.New, h.ticketSecret)
	_, _ = mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func deriveWsTicketSecret(authToken string) []byte {
	if authToken != "" {
		sum := sha256.Sum256([]byte("autorestic ws ticket:" + authToken))
		secret := make([]byte, sha256.Size)
		copy(secret, sum[:])
		return secret
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic("failed to initialize websocket ticket secret: " + err.Error())
	}
	return secret
}
