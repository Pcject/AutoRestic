package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/autorestic/autorestic/internal/model"
	_ "modernc.org/sqlite"
)

func setupLogStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE repositories (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);
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

func TestLogStoreGetByIDAppliesOutputTailLimit(t *testing.T) {
	db := setupLogStoreTestDB(t)
	defer db.Close()

	store := NewLogStore(db)
	stdout := strings.Repeat("0123456789", 30) + "\nlast-line\n"
	stderr := "warn\n"
	combined := stdout + stderr
	startedAt := time.Now().UTC()
	res, err := db.Exec(
		`INSERT INTO execution_logs (repo_id, task_id, command, stdout, stderr, combined_output, exit_code, status, trigger, started_at, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		1, 2, "restic snapshots --json", stdout, stderr, combined, 0, "success", "system_query", startedAt, 1234,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	logEntry, err := store.GetByID(id, 64)
	if err != nil {
		t.Fatal(err)
	}

	if !logEntry.StdoutMeta.Truncated {
		t.Fatal("expected stdout metadata to report truncation")
	}
	if logEntry.StdoutMeta.OriginalBytes != int64(len([]byte(stdout))) {
		t.Fatalf("expected original stdout bytes %d, got %d", len([]byte(stdout)), logEntry.StdoutMeta.OriginalBytes)
	}
	if logEntry.StdoutMeta.OriginalLines != model.CountOutputLines(stdout) {
		t.Fatalf("expected stdout line count %d, got %d", model.CountOutputLines(stdout), logEntry.StdoutMeta.OriginalLines)
	}
	if !strings.Contains(logEntry.Stdout, model.OutputTruncationMarkerPrefix) {
		t.Fatalf("expected stdout marker in %q", logEntry.Stdout)
	}
	if strings.Contains(logEntry.Stdout, strings.Repeat("0123456789", 10)) {
		t.Fatal("expected limited stdout to omit the earlier body and keep only the tail")
	}
	if !strings.Contains(logEntry.Stdout, "last-line") {
		t.Fatalf("expected stdout tail to include final line, got %q", logEntry.Stdout)
	}
	if logEntry.OutputLimit != 64 {
		t.Fatalf("expected output limit 64, got %d", logEntry.OutputLimit)
	}
}

func TestLogStoreGetByIDParsesStoredTruncationMarker(t *testing.T) {
	db := setupLogStoreTestDB(t)
	defer db.Close()

	store := NewLogStore(db)
	stdout := model.AppendOutputTruncationMarker("tail-contents\n", 4096, 512)
	res, err := db.Exec(
		`INSERT INTO execution_logs (command, stdout, stderr, combined_output, exit_code, status, trigger, started_at, duration_ms)
		 VALUES (?, ?, '', '', 0, 'success', 'system_query', ?, 1)`,
		"restic snapshots --json", stdout, time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	logEntry, err := store.GetByID(id, model.DefaultLogOutputLimit)
	if err != nil {
		t.Fatal(err)
	}
	if !logEntry.StdoutMeta.Truncated {
		t.Fatal("expected stored marker to be parsed as truncated output")
	}
	if logEntry.StdoutMeta.OriginalBytes != 4096 {
		t.Fatalf("expected original bytes 4096, got %d", logEntry.StdoutMeta.OriginalBytes)
	}
	if logEntry.StdoutMeta.OriginalLines != 512 {
		t.Fatalf("expected original lines 512, got %d", logEntry.StdoutMeta.OriginalLines)
	}
	if !strings.Contains(logEntry.Stdout, model.OutputTruncationMarkerPrefix) {
		t.Fatalf("expected truncation marker to be preserved in response, got %q", logEntry.Stdout)
	}
}

func TestLogStoreQueryReturnsListItemsWithoutOutputFields(t *testing.T) {
	db := setupLogStoreTestDB(t)
	defer db.Close()

	if _, err := db.Exec("INSERT INTO repositories (id, name) VALUES (1, 'repo-a')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO execution_logs (repo_id, command, stdout, stderr, combined_output, exit_code, status, trigger, started_at, duration_ms)
		 VALUES (1, 'restic backup /data', ?, ?, ?, 0, 'success', 'manual', ?, 99)`,
		strings.Repeat("x", 2048), "warn", "combined", time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}

	store := NewLogStore(db)
	result, err := store.Query(model.LogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}

	items, ok := result.Items.([]model.LogListItem)
	if !ok {
		t.Fatalf("expected []model.LogListItem, got %T", result.Items)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 log item, got %d", len(items))
	}
	if items[0].Command != "restic backup /data" {
		t.Fatalf("unexpected command %q", items[0].Command)
	}
}

func TestLogStoreQueryAppliesPagination(t *testing.T) {
	db := setupLogStoreTestDB(t)
	defer db.Close()

	base := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(
			`INSERT INTO execution_logs (command, stdout, stderr, combined_output, exit_code, status, trigger, started_at, duration_ms)
			 VALUES (?, '', '', '', 0, 'success', 'manual', ?, 1)`,
			fmt.Sprintf("cmd-%d", i), base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatal(err)
		}
	}

	store := NewLogStore(db)
	result, err := store.Query(model.LogQuery{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 5 || result.Page != 2 || result.Size != 2 {
		t.Fatalf("unexpected pagination metadata: %+v", result)
	}

	items, ok := result.Items.([]model.LogListItem)
	if !ok {
		t.Fatalf("expected []model.LogListItem, got %T", result.Items)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(items))
	}
	if items[0].Command != "cmd-3" || items[1].Command != "cmd-2" {
		t.Fatalf("expected page 2 commands cmd-3/cmd-2, got %+v", items)
	}
}

func TestLogStoreReportsElapsedDurationForRunningLogs(t *testing.T) {
	db := setupLogStoreTestDB(t)
	defer db.Close()

	startedAt := time.Now().UTC().Add(-2 * time.Minute)
	res, err := db.Exec(
		`INSERT INTO execution_logs (command, exit_code, status, trigger, started_at, duration_ms)
		 VALUES ('restic stats --json', -1, 'running', 'system_query', ?, 0)`,
		startedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	store := NewLogStore(db)
	result, err := store.Query(model.LogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	items := result.Items.([]model.LogListItem)
	if len(items) != 1 || items[0].DurationMs < 60_000 {
		t.Fatalf("expected running list item to report elapsed duration, got %+v", items)
	}

	logEntry, err := store.GetByID(id, model.DefaultLogOutputLimit)
	if err != nil {
		t.Fatal(err)
	}
	if logEntry.DurationMs < 60_000 {
		t.Fatalf("expected running detail to report elapsed duration, got %d", logEntry.DurationMs)
	}
}
