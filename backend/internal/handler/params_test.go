package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseInt64QueryRejectsMissingOrInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &gin.Context{}
	req := mustRequest(t, "/snapshots?repo_id=abc")
	c.Request = req

	if _, err := parseInt64Query(c, "repo_id"); err == nil {
		t.Fatal("expected invalid query id to return an error")
	}
}

func mustRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
