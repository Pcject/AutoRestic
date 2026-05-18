// backend/internal/executor/executor_test.go
package executor

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autorestic/autorestic/internal/model"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
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
			exit_code INTEGER NOT NULL DEFAULT -1,
			status TEXT NOT NULL DEFAULT 'running',
			trigger TEXT NOT NULL DEFAULT 'manual',
			started_at DATETIME NOT NULL DEFAULT (datetime('now')),
			finished_at DATETIME,
			duration_ms INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestExecutorRunEcho(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	exec := New(db, "echo")
	result := exec.Run(context.Background(), ExecRequest{
		Trigger: "manual",
		Args:    []string{"hello", "world"},
	})

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello world") {
		t.Fatalf("expected stdout to contain 'hello world', got: %q", result.Stdout)
	}

	var status string
	var command string
	err := db.QueryRow("SELECT status, command FROM execution_logs WHERE id = ?", result.LogID).Scan(&status, &command)
	if err != nil {
		t.Fatal(err)
	}
	if status != "success" {
		t.Fatalf("expected status 'success', got %q", status)
	}
}

func TestExecutorRunFailing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	exec := New(db, "false")
	result := exec.Run(context.Background(), ExecRequest{
		Trigger: "manual",
		Args:    []string{},
	})

	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}

	var status string
	db.QueryRow("SELECT status FROM execution_logs WHERE id = ?", result.LogID).Scan(&status)
	if status != "failed" {
		t.Fatalf("expected status 'failed', got %q", status)
	}
}

func TestRedactCommand(t *testing.T) {
	args := []string{"--repo", "/data", "--password=secret123"}
	redacted := redactCommand("restic", args)
	if strings.Contains(redacted, "secret123") {
		t.Fatalf("password was not redacted: %s", redacted)
	}
}

func TestExecutorStreamCallback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var lines []OutputLine
	exec := New(db, "echo")
	exec.Run(context.Background(), ExecRequest{
		Trigger: "manual",
		Args:    []string{"streaming", "test"},
		Callback: func(line OutputLine) {
			lines = append(lines, line)
		},
	})

	if len(lines) == 0 {
		t.Fatal("expected at least one streamed line")
	}
	if lines[0].Stream != "stdout" {
		t.Fatalf("expected stream 'stdout', got %q", lines[0].Stream)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestExecutorReadStreamHandlesLongLines(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	exec := New(db, "echo")
	var streamed []OutputLine
	var captured []string

	longLine := strings.Repeat("x", 70_000)
	err := exec.readStream(strings.NewReader(longLine+"\n"), "stdout", func(text string, hasNewline bool) {
		if hasNewline {
			captured = append(captured, text+"\n")
			return
		}
		captured = append(captured, text)
	}, func(line OutputLine) {
		streamed = append(streamed, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(streamed) != 1 || streamed[0].Text != longLine {
		t.Fatalf("expected one streamed long line, got %#v", streamed)
	}
	if len(captured) != 1 || !strings.Contains(captured[0], longLine) {
		t.Fatal("expected captured output to include long line")
	}
}

func TestExecutorReadStreamReturnsPipeError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	exec := New(db, "echo")
	var captured strings.Builder

	wantErr := errors.New("boom")
	err := exec.readStream(&errorReader{data: []byte("partial"), err: wantErr}, "stderr", func(text string, hasNewline bool) {
		captured.WriteString(text)
		if hasNewline || text != "" {
			captured.WriteString("\n")
		}
	}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	if got := captured.String(); got != "partial\n" {
		t.Fatalf("expected partial line to be captured, got %q", got)
	}
}

func TestExecutorRunSupportsCustomBinaryAndCommand(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tmp := t.TempDir()
	script := filepath.Join(tmp, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hook-ran\n"), 0700); err != nil {
		t.Fatal(err)
	}

	exec := New(db, "restic")
	result := exec.Run(context.Background(), ExecRequest{
		Trigger: "manual",
		Binary:  script,
		Command: "hook: echo hi",
	})
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	var command string
	if err := db.QueryRow("SELECT command FROM execution_logs WHERE id = ?", result.LogID).Scan(&command); err != nil {
		t.Fatal(err)
	}
	if command != "hook: echo hi" {
		t.Fatalf("expected custom command log, got %q", command)
	}
}

func TestExecutorRunSystemQueryTruncatesPersistedOutputs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tmp := t.TempDir()
	script := filepath.Join(tmp, "huge-output.sh")
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	builder.WriteString("i=1\n")
	builder.WriteString("while [ \"$i\" -le 12000 ]; do\n")
	builder.WriteString("  printf 'entry-%s\\n' \"$i\"\n")
	builder.WriteString("  i=$((i + 1))\n")
	builder.WriteString("done\n")
	if err := os.WriteFile(script, []byte(builder.String()), 0700); err != nil {
		t.Fatal(err)
	}

	exec := New(db, "restic")
	result := exec.Run(context.Background(), ExecRequest{
		Trigger: "system_query",
		Binary:  script,
	})
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	fullBytes := len([]byte(result.Stdout))
	if fullBytes <= storedOutputLimitBytes {
		t.Fatalf("expected stdout larger than stored limit, got %d", fullBytes)
	}
	if !strings.Contains(result.Stdout, "entry-12000") {
		t.Fatalf("expected full stdout to contain final line, got tail: %q", result.Stdout[len(result.Stdout)-64:])
	}

	var persistedStdout string
	var persistedCombined string
	var persistedBytes int
	if err := db.QueryRow(
		"SELECT stdout, combined_output, length(CAST(stdout AS BLOB)) FROM execution_logs WHERE id = ?",
		result.LogID,
	).Scan(&persistedStdout, &persistedCombined, &persistedBytes); err != nil {
		t.Fatal(err)
	}
	if persistedBytes >= fullBytes {
		t.Fatalf("expected persisted stdout smaller than full stdout, got persisted=%d full=%d", persistedBytes, fullBytes)
	}
	if !strings.Contains(persistedStdout, model.OutputTruncationMarkerPrefix) {
		t.Fatalf("expected stdout truncation marker, got %q", persistedStdout[len(persistedStdout)-128:])
	}
	if !strings.Contains(persistedCombined, model.OutputTruncationMarkerPrefix) {
		t.Fatalf("expected combined truncation marker, got %q", persistedCombined[len(persistedCombined)-128:])
	}

	meta, ok := model.ParseOutputTruncationMarker(persistedStdout)
	if !ok {
		t.Fatal("expected persisted stdout marker metadata")
	}
	if meta.OriginalBytes != int64(fullBytes) {
		t.Fatalf("expected original bytes %d, got %d", fullBytes, meta.OriginalBytes)
	}
	expectedLines := int64(12000)
	if meta.OriginalLines != expectedLines {
		t.Fatalf("expected %d lines, got %d", expectedLines, meta.OriginalLines)
	}
	if strings.Contains(persistedStdout, "entry-1\nentry-2\nentry-3") {
		t.Fatal("expected persisted stdout to keep tail rather than full output")
	}
	if !strings.Contains(persistedStdout, "entry-12000") {
		t.Fatal("expected persisted stdout tail to include final line")
	}
}

func TestBoundedOutputBufferTracksOriginalSizeAndLines(t *testing.T) {
	buf := newBoundedOutputBuffer(8)
	buf.WriteString("abc\n")
	buf.WriteString("def\n")
	buf.WriteString("ghi\n")

	got := buf.String()
	meta, ok := model.ParseOutputTruncationMarker(got)
	if !ok {
		t.Fatalf("expected truncation marker in %q", got)
	}
	if meta.OriginalBytes != int64(len([]byte("abc\ndef\nghi\n"))) {
		t.Fatalf("expected original byte count, got %d", meta.OriginalBytes)
	}
	if meta.OriginalLines != 3 {
		t.Fatalf("expected 3 lines, got %d", meta.OriginalLines)
	}
	if !strings.Contains(got, "ghi") {
		t.Fatalf("expected tail content to be kept, got %q", got)
	}
}

type errorReader struct {
	data []byte
	err  error
	read bool
}

func (r *errorReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		n := copy(p, r.data)
		return n, r.err
	}
	return 0, io.EOF
}
