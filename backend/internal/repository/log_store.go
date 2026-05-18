// backend/internal/repository/log_store.go
package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/autorestic/autorestic/internal/model"
)

type LogStore struct {
	db *sql.DB
}

type logFieldView struct {
	Value string
	Bytes int64
	Lines int64
}

func NewLogStore(db *sql.DB) *LogStore {
	return &LogStore{db: db}
}

func (s *LogStore) Query(q model.LogQuery) (*model.PaginatedResult, error) {
	where := []string{"1=1"}
	args := []any{}

	if q.RepoID != nil {
		where = append(where, "el.repo_id = ?")
		args = append(args, *q.RepoID)
	}
	if q.TaskID != nil {
		where = append(where, "el.task_id = ?")
		args = append(args, *q.TaskID)
	}
	if q.Status != "" {
		where = append(where, "el.status = ?")
		args = append(args, q.Status)
	}
	if q.Trigger != "" {
		where = append(where, "el.trigger = ?")
		args = append(args, q.Trigger)
	}
	if q.Command != "" {
		where = append(where, "el.command LIKE ?")
		args = append(args, "%"+q.Command+"%")
	}
	if q.Keyword != "" {
		where = append(where, "(el.stdout LIKE ? OR el.stderr LIKE ? OR el.command LIKE ?)")
		kw := "%" + q.Keyword + "%"
		args = append(args, kw, kw, kw)
	}

	whereClause := strings.Join(where, " AND ")

	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM execution_logs el WHERE %s", whereClause)
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 50
	}
	offset := (q.Page - 1) * q.PageSize

	querySQL := fmt.Sprintf(`
		SELECT el.id, el.repo_id, COALESCE(r.name, ''), el.task_id, el.command,
		       el.exit_code, el.status, el.trigger, el.started_at, el.finished_at, el.duration_ms
		FROM execution_logs el
		LEFT JOIN repositories r ON el.repo_id = r.id
		WHERE %s
		ORDER BY el.started_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs := append(args, q.PageSize, offset)
	rows, err := s.db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.LogListItem
	for rows.Next() {
		var item model.LogListItem
		if err := rows.Scan(&item.ID, &item.RepoID, &item.RepoName, &item.TaskID,
			&item.Command, &item.ExitCode, &item.Status, &item.Trigger,
			&item.StartedAt, &item.FinishedAt, &item.DurationMs); err != nil {
			return nil, err
		}
		item.DurationMs = visibleDurationMs(item.Status, item.StartedAt, item.DurationMs)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.PaginatedResult{
		Items: items,
		Total: total,
		Page:  q.Page,
		Size:  q.PageSize,
	}, nil
}

func (s *LogStore) GetByID(id int64, outputLimit int) (*model.ExecutionLog, error) {
	outputLimit = model.ClampLogOutputLimit(outputLimit)

	var log model.ExecutionLog
	var stdout logFieldView
	var stderr logFieldView
	var combined logFieldView
	err := s.db.QueryRow(
		`SELECT id, repo_id, task_id, command,
		        CASE WHEN length(CAST(stdout AS BLOB)) > ? THEN substr(stdout, -?) ELSE stdout END,
		        length(CAST(stdout AS BLOB)),
		        CASE WHEN stdout = '' THEN 0 ELSE length(stdout) - length(replace(stdout, char(10), '')) END,
		        CASE WHEN length(CAST(stderr AS BLOB)) > ? THEN substr(stderr, -?) ELSE stderr END,
		        length(CAST(stderr AS BLOB)),
		        CASE WHEN stderr = '' THEN 0 ELSE length(stderr) - length(replace(stderr, char(10), '')) END,
		        CASE WHEN length(CAST(combined_output AS BLOB)) > ? THEN substr(combined_output, -?) ELSE combined_output END,
		        length(CAST(combined_output AS BLOB)),
		        CASE WHEN combined_output = '' THEN 0 ELSE length(combined_output) - length(replace(combined_output, char(10), '')) END,
		        exit_code, status, trigger, started_at, finished_at, duration_ms
		 FROM execution_logs WHERE id = ?`, outputLimit, outputLimit, outputLimit, outputLimit, outputLimit, outputLimit, id,
	).Scan(&log.ID, &log.RepoID, &log.TaskID, &log.Command,
		&stdout.Value, &stdout.Bytes, &stdout.Lines,
		&stderr.Value, &stderr.Bytes, &stderr.Lines,
		&combined.Value, &combined.Bytes, &combined.Lines,
		&log.ExitCode, &log.Status, &log.Trigger,
		&log.StartedAt, &log.FinishedAt, &log.DurationMs)
	if err != nil {
		return nil, err
	}
	log.Stdout, log.StdoutMeta = normalizeLogOutput(stdout)
	log.Stderr, log.StderrMeta = normalizeLogOutput(stderr)
	log.CombinedOutput, log.CombinedMeta = normalizeLogOutput(combined)
	log.DurationMs = visibleDurationMs(log.Status, log.StartedAt, log.DurationMs)
	log.OutputLimit = outputLimit
	return &log, nil
}

func visibleDurationMs(status string, startedAt time.Time, stored int64) int64 {
	if status != "running" {
		return stored
	}
	elapsed := time.Since(startedAt).Milliseconds()
	if elapsed > stored {
		return elapsed
	}
	return stored
}

func (s *LogStore) GetStatusByID(id int64) (string, error) {
	var status string
	err := s.db.QueryRow("SELECT status FROM execution_logs WHERE id = ?", id).Scan(&status)
	if err != nil {
		return "", err
	}
	return status, nil
}

func (s *LogStore) DeleteOlderThan(days int) (int64, error) {
	res, err := s.db.Exec(
		"DELETE FROM execution_logs WHERE started_at < datetime('now', ? || ' days')",
		fmt.Sprintf("-%d", days),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func normalizeLogOutput(field logFieldView) (string, model.ExecutionOutputMeta) {
	meta := model.ExecutionOutputMeta{
		OriginalBytes: field.Bytes,
		OriginalLines: field.Lines,
	}
	if field.Value == "" && field.Bytes == 0 && field.Lines == 0 {
		return "", meta
	}

	if markerMeta, ok := model.ParseOutputTruncationMarker(field.Value); ok {
		meta = markerMeta
		return field.Value, meta
	}

	if meta.OriginalBytes == 0 {
		meta.OriginalBytes = int64(len([]byte(field.Value)))
	}
	if meta.OriginalLines == 0 {
		meta.OriginalLines = model.CountOutputLines(field.Value)
	}
	if int64(len([]byte(field.Value))) < meta.OriginalBytes {
		meta.Truncated = true
		return model.AppendOutputTruncationMarker(field.Value, meta.OriginalBytes, meta.OriginalLines), meta
	}
	return field.Value, meta
}
