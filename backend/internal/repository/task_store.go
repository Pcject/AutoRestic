package repository

import (
	"database/sql"

	"github.com/autorestic/autorestic/internal/model"
)

type TaskStore struct {
	db *sql.DB
}

const taskColumns = `id, repo_id, name, source_paths, excludes, tags,
	cron_expr, cron_enabled, forget_policy, pre_hooks, post_hooks,
	extra_flags, created_at, updated_at, last_run_at, next_run_at`

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

func (s *TaskStore) List(q model.TaskQuery) ([]model.BackupTask, error) {
	where := "1=1"
	args := []any{}

	if q.RepoID != nil {
		where += " AND repo_id = ?"
		args = append(args, *q.RepoID)
	}
	if q.CronEnabled != nil {
		where += " AND cron_enabled = ?"
		if *q.CronEnabled {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}

	rows, err := s.db.Query("SELECT "+taskColumns+" FROM backup_tasks WHERE "+where+" ORDER BY created_at DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.BackupTask
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *TaskStore) GetByID(id int64) (*model.BackupTask, error) {
	t, err := scanTask(s.db.QueryRow("SELECT "+taskColumns+" FROM backup_tasks WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *TaskStore) Create(t *model.BackupTask) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO backup_tasks (repo_id, name, source_paths, excludes, tags, cron_expr,
		  cron_enabled, forget_policy, pre_hooks, post_hooks, extra_flags, next_run_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.RepoID, t.Name, t.SourcePaths, t.Excludes, t.Tags, t.CronExpr,
		t.CronEnabled, t.ForgetPolicy, t.PreHooks, t.PostHooks, t.ExtraFlags, t.NextRunAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *TaskStore) Update(t *model.BackupTask) error {
	_, err := s.db.Exec(
		`UPDATE backup_tasks SET name=?, source_paths=?, excludes=?, tags=?, cron_expr=?,
		  cron_enabled=?, forget_policy=?, pre_hooks=?, post_hooks=?, extra_flags=?,
		  updated_at=datetime('now'), next_run_at=? WHERE id=?`,
		t.Name, t.SourcePaths, t.Excludes, t.Tags, t.CronExpr, t.CronEnabled,
		t.ForgetPolicy, t.PreHooks, t.PostHooks, t.ExtraFlags, t.NextRunAt, t.ID)
	return err
}

func (s *TaskStore) UpdateRunTimes(id int64, lastRun, nextRun any) error {
	_, err := s.db.Exec(
		"UPDATE backup_tasks SET last_run_at=?, next_run_at=?, updated_at=datetime('now') WHERE id=?",
		lastRun, nextRun, id)
	return err
}

func (s *TaskStore) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM backup_tasks WHERE id = ?", id)
	return err
}

func (s *TaskStore) ListEnabled() ([]model.BackupTask, error) {
	rows, err := s.db.Query(
		"SELECT " + taskColumns + " FROM backup_tasks WHERE cron_enabled = 1 AND cron_expr != '' ORDER BY next_run_at ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.BackupTask
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (model.BackupTask, error) {
	var t model.BackupTask
	err := scanner.Scan(&t.ID, &t.RepoID, &t.Name, &t.SourcePaths, &t.Excludes, &t.Tags,
		&t.CronExpr, &t.CronEnabled, &t.ForgetPolicy, &t.PreHooks, &t.PostHooks,
		&t.ExtraFlags, &t.CreatedAt, &t.UpdatedAt, &t.LastRunAt, &t.NextRunAt)
	return t, err
}
