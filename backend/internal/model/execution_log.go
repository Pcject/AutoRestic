package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultLogOutputLimit = 64 * 1024
	MaxLogOutputLimit     = 256 * 1024

	OutputTruncationMarkerPrefix = "[autorestic-output-truncated]"
)

var outputTruncationMarkerPattern = regexp.MustCompile(`(?m)\[autorestic-output-truncated\] original_bytes=(\d+) original_lines=(\d+)\n?$`)

type ExecutionOutputMeta struct {
	Truncated     bool  `json:"truncated"`
	OriginalBytes int64 `json:"original_bytes"`
	OriginalLines int64 `json:"original_lines"`
}

type ExecutionLog struct {
	ID             int64               `json:"id"`
	RepoID         *int64              `json:"repo_id"`
	TaskID         *int64              `json:"task_id"`
	Command        string              `json:"command"`
	Stdout         string              `json:"stdout,omitempty"`
	Stderr         string              `json:"stderr,omitempty"`
	CombinedOutput string              `json:"combined_output,omitempty"`
	ExitCode       int                 `json:"exit_code"`
	Status         string              `json:"status"`
	Trigger        string              `json:"trigger"`
	StartedAt      time.Time           `json:"started_at"`
	FinishedAt     *time.Time          `json:"finished_at"`
	DurationMs     int64               `json:"duration_ms"`
	StdoutMeta     ExecutionOutputMeta `json:"stdout_meta"`
	StderrMeta     ExecutionOutputMeta `json:"stderr_meta"`
	CombinedMeta   ExecutionOutputMeta `json:"combined_output_meta"`
	OutputLimit    int                 `json:"output_limit,omitempty"`
}

type LogQuery struct {
	RepoID   *int64 `form:"repo_id"`
	TaskID   *int64 `form:"task_id"`
	Status   string `form:"status"`
	Trigger  string `form:"trigger"`
	Command  string `form:"command"`
	Keyword  string `form:"keyword"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=50"`
}

type LogListItem struct {
	ID         int64      `json:"id"`
	RepoID     *int64     `json:"repo_id"`
	RepoName   string     `json:"repo_name"`
	TaskID     *int64     `json:"task_id"`
	Command    string     `json:"command"`
	ExitCode   int        `json:"exit_code"`
	Status     string     `json:"status"`
	Trigger    string     `json:"trigger"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMs int64      `json:"duration_ms"`
}

type PaginatedResult struct {
	Items any   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"page_size"`
}

func ClampLogOutputLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultLogOutputLimit
	case limit > MaxLogOutputLimit:
		return MaxLogOutputLimit
	default:
		return limit
	}
}

func CountOutputLines(text string) int64 {
	if text == "" {
		return 0
	}
	lines := int64(strings.Count(text, "\n"))
	if strings.HasSuffix(text, "\n") {
		return lines
	}
	return lines + 1
}

func AppendOutputTruncationMarker(text string, originalBytes, originalLines int64) string {
	if originalBytes < 0 {
		originalBytes = 0
	}
	if originalLines < 0 {
		originalLines = 0
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + fmt.Sprintf("%s original_bytes=%d original_lines=%d\n", OutputTruncationMarkerPrefix, originalBytes, originalLines)
}

func ParseOutputTruncationMarker(text string) (ExecutionOutputMeta, bool) {
	matches := outputTruncationMarkerPattern.FindStringSubmatch(text)
	if len(matches) != 3 {
		return ExecutionOutputMeta{}, false
	}
	originalBytes, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return ExecutionOutputMeta{}, false
	}
	originalLines, err := strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return ExecutionOutputMeta{}, false
	}
	return ExecutionOutputMeta{
		Truncated:     true,
		OriginalBytes: originalBytes,
		OriginalLines: originalLines,
	}, true
}
