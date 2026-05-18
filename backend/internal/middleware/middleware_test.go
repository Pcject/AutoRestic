package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupMiddlewareRequiresBearerTokenWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupMiddleware(r, Options{
		AuthToken: "secret",
	})
	r.GET("/api/v1/repos", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without token, got %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected ok with token, got %d", resp.Code)
	}
}

func TestSetupMiddlewareDelegatesWebSocketAuthToHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupMiddleware(r, Options{
		AuthToken: "secret",
	})
	r.GET("/ws/executions/:id", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ws/executions/1", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected websocket path to be delegated to handler, got %d", resp.Code)
	}
}
