package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRepoCheckRejectsSensitiveGETQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	api := router.Group("/api/v1")
	NewRepoHandler(nil).Register(api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos/check?path=/repo&type=local&password=secret", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "secret") {
		t.Fatal("response should not echo query credentials")
	}
}
