package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/repository"
	"github.com/autorestic/autorestic/internal/service"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func setupLogHandlerTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE execution_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id INTEGER,
			task_id INTEGER,
			command TEXT NOT NULL,
			stdout TEXT NOT NULL DEFAULT '',
			stderr TEXT NOT NULL DEFAULT '',
			combined_output TEXT NOT NULL DEFAULT '',
			exit_code INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'success',
			trigger TEXT NOT NULL DEFAULT 'manual',
			started_at DATETIME NOT NULL,
			finished_at DATETIME,
			duration_ms INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLogHandlerGetAppliesDefaultOutputLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupLogHandlerTestDB(t)
	defer db.Close()

	stdout := strings.Repeat("1234567890", 9000) + "\nfinal-line\n"
	if _, err := db.Exec(
		`INSERT INTO execution_logs (command, stdout, stderr, combined_output, exit_code, status, trigger, started_at, duration_ms)
		 VALUES (?, ?, '', '', 0, 'success', 'system_query', ?, 1)`,
		"restic snapshots --json", stdout, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}

	logSvc := service.NewLogService(repository.NewLogStore(db), executor.New(db, "restic"))
	logHandler := NewLogHandler(logSvc)
	router := gin.New()
	api := router.Group("")
	logHandler.Register(api)

	req := httptest.NewRequest(http.MethodGet, "/logs/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var response model.ExecutionLog
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.OutputLimit != model.DefaultLogOutputLimit {
		t.Fatalf("expected default output limit %d, got %d", model.DefaultLogOutputLimit, response.OutputLimit)
	}
	if !response.StdoutMeta.Truncated {
		t.Fatal("expected stdout to be truncated by default detail limit")
	}
	if !strings.Contains(response.Stdout, model.OutputTruncationMarkerPrefix) {
		t.Fatalf("expected truncation marker in stdout, got %q", response.Stdout)
	}
	if !strings.Contains(response.Stdout, "final-line") {
		t.Fatalf("expected stdout tail to include final line, got %q", response.Stdout)
	}
}

func TestLogHandlerGetRejectsInvalidLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupLogHandlerTestDB(t)
	defer db.Close()

	if _, err := db.Exec(
		`INSERT INTO execution_logs (command, stdout, stderr, combined_output, exit_code, status, trigger, started_at, duration_ms)
		 VALUES ('restic snapshots --json', '', '', '', 0, 'success', 'system_query', ?, 1)`,
		time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}

	logSvc := service.NewLogService(repository.NewLogStore(db), executor.New(db, "restic"))
	logHandler := NewLogHandler(logSvc)
	router := gin.New()
	api := router.Group("")
	logHandler.Register(api)

	req := httptest.NewRequest(http.MethodGet, "/logs/1?limit=abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
