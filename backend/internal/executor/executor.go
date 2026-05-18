// backend/internal/executor/executor.go
package executor

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/ws"
)

// OutputLine represents a single line of output with metadata.
type OutputLine struct {
	Time   time.Time `json:"time"`
	Stream string    `json:"stream"` // "stdout" or "stderr"
	Text   string    `json:"text"`
}

// StreamCallback is called for each output line in real-time.
type StreamCallback func(line OutputLine)

// ExecRequest describes a restic command to execute.
type ExecRequest struct {
	RepoID   *int64
	TaskID   *int64
	Trigger  string // manual, scheduled, system_query
	Binary   string
	Args     []string
	Env      []string // extra env vars like RESTIC_REPOSITORY, RESTIC_PASSWORD
	Command  string
	Callback StreamCallback
	Hub      *ws.Hub
	Started  chan<- int64
}

// ExecResult is returned after execution completes.
type ExecResult struct {
	LogID          int64
	ExitCode       int
	Stdout         string
	Stderr         string
	CombinedOutput string
	Duration       time.Duration
	Err            error
}

// Executor runs restic CLI commands and logs everything.
type Executor struct {
	db           *sql.DB
	resticBin    string
	mu           sync.Mutex
	activeCancel map[int64]context.CancelFunc
}

const storedOutputLimitBytes = 128 * 1024

// New creates a new Executor.
func New(db *sql.DB, resticBin string) *Executor {
	return &Executor{db: db, resticBin: resticBin, activeCancel: map[int64]context.CancelFunc{}}
}

var sensitivePattern = regexp.MustCompile(`(?i)(password|passwd|RESTIC_PASSWORD)=\S+`)

func redactCommand(binary string, args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = sensitivePattern.ReplaceAllString(a, "${1}=***")
	}
	return filepath.Base(binary) + " " + strings.Join(parts, " ")
}

// Run executes a restic command and records everything.
func (e *Executor) Run(ctx context.Context, req ExecRequest) ExecResult {
	startedAt := time.Now()

	// Insert initial log record
	binary := req.Binary
	if strings.TrimSpace(binary) == "" {
		binary = e.resticBin
	}
	redacted := req.Command
	if strings.TrimSpace(redacted) == "" {
		redacted = redactCommand(binary, req.Args)
	}
	logID, err := e.insertLog(req, redacted, startedAt)
	if err != nil {
		return ExecResult{Err: fmt.Errorf("insert log: %w", err)}
	}

	runCtx, cancel := context.WithCancel(ctx)
	e.registerCancel(logID, cancel)
	defer func() {
		cancel()
		e.unregisterCancel(logID)
	}()

	// Build command
	cmd := exec.CommandContext(runCtx, binary, req.Args...)
	cmd.Env = append(cmd.Environ(), req.Env...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		e.finishLog(logID, "", "", "", -1, startedAt, "failed")
		return ExecResult{LogID: logID, ExitCode: -1, Err: err}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		e.finishLog(logID, "", "", "", -1, startedAt, "failed")
		return ExecResult{LogID: logID, ExitCode: -1, Err: err}
	}

	if err := cmd.Start(); err != nil {
		e.finishLog(logID, "", "", "", -1, startedAt, "failed")
		return ExecResult{LogID: logID, ExitCode: -1, Err: err}
	}
	if req.Started != nil {
		req.Started <- logID
	}

	// Read stdout and stderr concurrently
	var (
		outputs = newCommandOutputBuffers(req.Trigger != "system_query")
		mu      sync.Mutex
	)

	var wg sync.WaitGroup
	type pipeReadResult struct {
		stream string
		err    error
	}
	readPipe := func(reader io.Reader, stream string, results chan<- pipeReadResult) {
		defer wg.Done()
		if err := e.readStream(reader, stream, func(text string, hasNewline bool) {
			mu.Lock()
			outputs.appendLine(stream, text, hasNewline)
			mu.Unlock()
		}, func(line OutputLine) {
			if req.Callback != nil {
				req.Callback(line)
			}
			if req.Hub != nil {
				req.Hub.SendOutput(logID, line.Stream, line.Text)
			}
		}); err != nil {
			results <- pipeReadResult{stream: stream, err: err}
		}
	}

	readResults := make(chan pipeReadResult, 2)
	wg.Add(2)
	go readPipe(stdoutPipe, "stdout", readResults)
	go readPipe(stderrPipe, "stderr", readResults)
	wg.Wait()
	close(readResults)

	exitCode := 0
	status := "success"
	waitErr := cmd.Wait()
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		status = "failed"
		if runCtx.Err() != nil {
			exitCode = -1
			status = "cancelled"
		}
	}

	var pipeErr error
	for readResult := range readResults {
		if readResult.err == nil {
			continue
		}
		wrapped := fmt.Errorf("%s pipe read: %w", readResult.stream, readResult.err)
		if pipeErr == nil {
			pipeErr = wrapped
		} else {
			pipeErr = errors.Join(pipeErr, wrapped)
		}
	}

	// Send WebSocket completion message
	if req.Hub != nil {
		finalExitCode := exitCode
		if pipeErr != nil && finalExitCode == 0 {
			finalExitCode = -1
		}
		req.Hub.SendComplete(logID, finalExitCode)
	}

	if pipeErr != nil {
		pipeText := pipeErr.Error()
		mu.Lock()
		outputs.appendPipeError(pipeText)
		mu.Unlock()
		status = "failed"
		if exitCode == 0 {
			exitCode = -1
		}
	}

	stdout, stderr, combined := outputs.result()
	e.finishLog(logID, outputs.persistedStdout(), outputs.persistedStderr(), outputs.persistedCombined(), exitCode, startedAt, status)

	result := ExecResult{
		LogID:          logID,
		ExitCode:       exitCode,
		Stdout:         stdout,
		Stderr:         stderr,
		CombinedOutput: combined,
		Duration:       time.Since(startedAt),
	}
	if pipeErr != nil {
		result.Err = pipeErr
	}
	return result
}

func (e *Executor) readStream(reader io.Reader, stream string, record func(text string, hasNewline bool), emit func(OutputLine)) error {
	buffered := bufio.NewReader(reader)
	for {
		line, err := buffered.ReadString('\n')
		if len(line) > 0 {
			hasNewline := strings.HasSuffix(line, "\n")
			text := strings.TrimSuffix(line, "\n")
			now := time.Now()
			if record != nil {
				record(text, hasNewline)
			}

			if emit != nil {
				emit(OutputLine{Time: now, Stream: stream, Text: text})
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

type boundedOutputBuffer struct {
	limit         int
	tail          []byte
	originalBytes int64
	originalLines int64
	truncated     bool
}

func newBoundedOutputBuffer(limit int) *boundedOutputBuffer {
	return &boundedOutputBuffer{limit: limit}
}

func (b *boundedOutputBuffer) WriteString(text string) {
	if text == "" {
		return
	}
	b.originalBytes += int64(len([]byte(text)))
	b.originalLines += model.CountOutputLines(text)
	if b.limit <= 0 {
		b.tail = append(b.tail, text...)
		return
	}
	b.tail = append(b.tail, text...)
	if len(b.tail) <= b.limit {
		return
	}
	b.truncated = true
	b.tail = append([]byte(nil), b.tail[len(b.tail)-b.limit:]...)
}

func (b *boundedOutputBuffer) String() string {
	if !b.truncated {
		return string(b.tail)
	}
	return model.AppendOutputTruncationMarker(string(b.tail), b.originalBytes, b.originalLines)
}

type commandOutputBuffers struct {
	fullCombined bool

	stdoutResult   strings.Builder
	stderrResult   strings.Builder
	combinedResult strings.Builder

	stdoutLog   *boundedOutputBuffer
	stderrLog   *boundedOutputBuffer
	combinedLog *boundedOutputBuffer
}

func newCommandOutputBuffers(fullCombined bool) *commandOutputBuffers {
	return &commandOutputBuffers{
		fullCombined: fullCombined,
		stdoutLog:    newBoundedOutputBuffer(storedOutputLimitBytes),
		stderrLog:    newBoundedOutputBuffer(storedOutputLimitBytes),
		combinedLog:  newBoundedOutputBuffer(storedOutputLimitBytes),
	}
}

func (b *commandOutputBuffers) appendLine(stream, text string, hasNewline bool) {
	normalized := text
	if hasNewline {
		normalized += "\n"
	} else if normalized != "" {
		normalized += "\n"
	}
	switch stream {
	case "stdout":
		b.stdoutResult.WriteString(normalized)
		b.stdoutLog.WriteString(normalized)
	case "stderr":
		b.stderrResult.WriteString(normalized)
		b.stderrLog.WriteString(normalized)
	}

	combinedLine := fmt.Sprintf("[%s] %s\n", stream, text)
	if b.fullCombined {
		b.combinedResult.WriteString(combinedLine)
	}
	b.combinedLog.WriteString(combinedLine)
}

func (b *commandOutputBuffers) appendPipeError(pipeText string) {
	normalized := pipeText
	if normalized != "" && !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	b.stderrResult.WriteString(normalized)
	b.stderrLog.WriteString(normalized)

	combinedLine := fmt.Sprintf("[error] %s\n", pipeText)
	if b.fullCombined {
		b.combinedResult.WriteString(combinedLine)
	}
	b.combinedLog.WriteString(combinedLine)
}

func (b *commandOutputBuffers) result() (string, string, string) {
	combined := b.combinedResult.String()
	if !b.fullCombined {
		combined = b.combinedLog.String()
	}
	return b.stdoutResult.String(), b.stderrResult.String(), combined
}

func (b *commandOutputBuffers) persistedStdout() string {
	return b.stdoutLog.String()
}

func (b *commandOutputBuffers) persistedStderr() string {
	return b.stderrLog.String()
}

func (b *commandOutputBuffers) persistedCombined() string {
	return b.combinedLog.String()
}

func (e *Executor) Cancel(logID int64) bool {
	e.mu.Lock()
	cancel, ok := e.activeCancel[logID]
	e.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

func (e *Executor) registerCancel(logID int64, cancel context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.activeCancel[logID] = cancel
}

func (e *Executor) unregisterCancel(logID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.activeCancel, logID)
}

func (e *Executor) insertLog(req ExecRequest, command string, startedAt time.Time) (int64, error) {
	res, err := e.db.Exec(
		`INSERT INTO execution_logs (repo_id, task_id, command, trigger, status, started_at)
		 VALUES (?, ?, ?, ?, 'running', ?)`,
		req.RepoID, req.TaskID, command, req.Trigger, startedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (e *Executor) finishLog(logID int64, stdout, stderr, combined string, exitCode int, startedAt time.Time, status string) {
	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	e.db.Exec(
		`UPDATE execution_logs SET stdout=?, stderr=?, combined_output=?, exit_code=?,
		 status=?, finished_at=?, duration_ms=? WHERE id=?`,
		stdout, stderr, combined, exitCode, status, finishedAt, durationMs, logID,
	)
}
