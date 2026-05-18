package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/repository"
	_ "modernc.org/sqlite"
)

func setupTaskServiceTest(t *testing.T) (*TaskService, *repository.RepoStore, *repository.TaskStore, string) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE repositories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			password_encrypted TEXT NOT NULL DEFAULT '',
			rclone_config TEXT NOT NULL DEFAULT '',
			rclone_config_encrypted TEXT NOT NULL DEFAULT '',
			webdav_url TEXT NOT NULL DEFAULT '',
			webdav_user TEXT NOT NULL DEFAULT '',
			webdav_password_encrypted TEXT NOT NULL DEFAULT '',
			options TEXT NOT NULL DEFAULT '{}',
			prune_enabled INTEGER NOT NULL DEFAULT 1,
			prune_cron_expr TEXT NOT NULL DEFAULT '0 3 * * 0',
			prune_args TEXT NOT NULL DEFAULT '[]',
			check_enabled INTEGER NOT NULL DEFAULT 1,
			check_cron_expr TEXT NOT NULL DEFAULT '0 4 1 * *',
			check_args TEXT NOT NULL DEFAULT '["--read-data-subset=10%"]',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);
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
		);
		CREATE TABLE backup_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			source_paths TEXT NOT NULL DEFAULT '[]',
			excludes TEXT NOT NULL DEFAULT '[]',
			tags TEXT NOT NULL DEFAULT '[]',
			cron_expr TEXT NOT NULL DEFAULT '',
			cron_enabled INTEGER NOT NULL DEFAULT 0,
			forget_policy TEXT NOT NULL DEFAULT '{}',
			pre_hooks TEXT NOT NULL DEFAULT '[]',
			post_hooks TEXT NOT NULL DEFAULT '[]',
			extra_flags TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			last_run_at DATETIME,
			next_run_at DATETIME
		);
		CREATE TABLE repository_cache (
			repo_id INTEGER NOT NULL,
			cache_key TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '',
			refreshed_at DATETIME NOT NULL DEFAULT (datetime('now')),
			error TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (repo_id, cache_key),
			FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE repository_snapshots (
			repo_id INTEGER NOT NULL,
			snapshot_id TEXT NOT NULL,
			short_id TEXT NOT NULL DEFAULT '',
			time TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			uid INTEGER NOT NULL DEFAULT 0,
			gid INTEGER NOT NULL DEFAULT 0,
			tags TEXT NOT NULL DEFAULT '[]',
			paths TEXT NOT NULL DEFAULT '[]',
			tree TEXT NOT NULL DEFAULT '',
			program_version TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '{}',
			backup_start TEXT NOT NULL DEFAULT '',
			backup_end TEXT NOT NULL DEFAULT '',
			files_new INTEGER NOT NULL DEFAULT 0,
			files_changed INTEGER NOT NULL DEFAULT 0,
			files_unmodified INTEGER NOT NULL DEFAULT 0,
			dirs_new INTEGER NOT NULL DEFAULT 0,
			dirs_changed INTEGER NOT NULL DEFAULT 0,
			dirs_unmodified INTEGER NOT NULL DEFAULT 0,
			data_blobs INTEGER NOT NULL DEFAULT 0,
			tree_blobs INTEGER NOT NULL DEFAULT 0,
			data_added INTEGER NOT NULL DEFAULT 0,
			data_added_packed INTEGER NOT NULL DEFAULT 0,
			total_files_processed INTEGER NOT NULL DEFAULT 0,
			total_bytes_processed INTEGER NOT NULL DEFAULT 0,
			raw_json TEXT NOT NULL DEFAULT '{}',
			indexed_at DATETIME NOT NULL DEFAULT (datetime('now')),
			generation INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (repo_id, snapshot_id, generation),
			FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE repository_sync_state (
			repo_id INTEGER NOT NULL,
			domain TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'stale',
			phase TEXT NOT NULL DEFAULT '',
			progress INTEGER NOT NULL DEFAULT 0,
			generation INTEGER NOT NULL DEFAULT 0,
			last_success_at DATETIME,
			last_error TEXT NOT NULL DEFAULT '',
			log_id INTEGER,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (repo_id, domain)
		);
		CREATE TABLE repository_stats (
			repo_id INTEGER NOT NULL,
			generation INTEGER NOT NULL DEFAULT 0,
			total_size INTEGER NOT NULL DEFAULT 0,
			total_file_count INTEGER NOT NULL DEFAULT 0,
			total_blob_count INTEGER NOT NULL DEFAULT 0,
			snapshot_count INTEGER NOT NULL DEFAULT 0,
			raw_json TEXT NOT NULL DEFAULT '{}',
			indexed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (repo_id, generation)
		);
		CREATE TABLE repository_keys (
			repo_id INTEGER NOT NULL,
			generation INTEGER NOT NULL DEFAULT 0,
			key_id TEXT NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			created TEXT NOT NULL DEFAULT '',
			expires TEXT NOT NULL DEFAULT '',
			current INTEGER NOT NULL DEFAULT 0,
			raw_json TEXT NOT NULL DEFAULT '{}',
			indexed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (repo_id, generation, key_id)
		);
		CREATE TABLE repository_snapshot_files (
			repo_id INTEGER NOT NULL,
			snapshot_id TEXT NOT NULL,
			path TEXT NOT NULL,
			parent_path TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT '',
			size INTEGER NOT NULL DEFAULT 0,
			mode TEXT NOT NULL DEFAULT '',
			mtime TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '{}',
			generation INTEGER NOT NULL DEFAULT 0,
			indexed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (repo_id, snapshot_id, generation, path)
		);
		CREATE TABLE repository_snapshot_file_indexes (
			repo_id INTEGER NOT NULL,
			snapshot_id TEXT NOT NULL,
			path TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'stale',
			entry_count INTEGER NOT NULL DEFAULT 0,
			indexed_at DATETIME,
			last_error TEXT NOT NULL DEFAULT '',
			stale INTEGER NOT NULL DEFAULT 1,
			generation INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (repo_id, snapshot_id, path)
		);
		CREATE TABLE repository_restic_config (
			repo_id INTEGER NOT NULL,
			generation INTEGER NOT NULL DEFAULT 0,
			repository_id TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 0,
			raw_json TEXT NOT NULL DEFAULT '{}',
			indexed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (repo_id, generation)
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "restic-args.log")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	exec := executor.New(db, resticBin)
	repoStore := repository.NewRepoStore(db)
	taskStore := repository.NewTaskStore(db)
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	return NewTaskService(taskStore, repoSvc), repoStore, taskStore, logPath
}

func TestRunTaskIncludesStoredExtraFlags(t *testing.T) {
	taskSvc, repoStore, taskStore, logPath := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "daily",
		SourcePaths:  `["/data"]`,
		Excludes:     `["*.tmp"]`,
		Tags:         `["daily"]`,
		ForgetPolicy: "{}",
		PreHooks:     "[]",
		PostHooks:    "[]",
		ExtraFlags:   `{"--host":"nas","--dry-run":true,"--limit-upload":512}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := taskSvc.RunTask(context.Background(), taskID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(content)
	for _, want := range []string{"backup", "--host", "nas", "--dry-run", "--limit-upload", "512", "--json", "--exclude", "*.tmp", "--tag", "daily", "/data"} {
		if !strings.Contains(args, want) {
			t.Fatalf("expected args to contain %q, got %q", want, args)
		}
	}
}

func TestCreateTaskPersistsPolicyHooksAndExtraFlags(t *testing.T) {
	taskSvc, repoStore, _, _ := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskSvc.Create(model.CreateTaskRequest{
		RepoID:       repoID,
		Name:         "daily",
		SourcePaths:  `["/data"]`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: `{"keep-last":7}`,
		PreHooks:     `["echo before"]`,
		PostHooks:    `["echo after"]`,
		ExtraFlags:   `{"--dry-run":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := taskSvc.GetByID(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.ForgetPolicy != `{"keep-last":7}` {
		t.Fatalf("forget policy was not persisted: %q", task.ForgetPolicy)
	}
	if task.PreHooks != `["echo before"]` {
		t.Fatalf("pre hooks were not persisted: %q", task.PreHooks)
	}
	if task.PostHooks != `["echo after"]` {
		t.Fatalf("post hooks were not persisted: %q", task.PostHooks)
	}
	if task.ExtraFlags != `{"--dry-run":true}` {
		t.Fatalf("extra flags were not persisted: %q", task.ExtraFlags)
	}
}

func TestCreateTaskDefaultsToExplicitUnlimitedKeepLast(t *testing.T) {
	taskSvc, repoStore, _, logPath := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskSvc.Create(model.CreateTaskRequest{
		RepoID:      repoID,
		Name:        "daily",
		SourcePaths: `["/data"]`,
		Excludes:    `[]`,
		Tags:        `[]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := taskSvc.GetByID(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.ForgetPolicy != `{"keep-last":"unlimited"}` {
		t.Fatalf("expected explicit unlimited keep-last default, got %q", task.ForgetPolicy)
	}

	if _, err := taskSvc.RunTask(context.Background(), taskID, "manual"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "forget --keep-last unlimited") {
		t.Fatalf("expected unlimited forget command, got %q", string(content))
	}
}

func TestRunTaskRejectsInvalidTaskJSON(t *testing.T) {
	taskSvc, repoStore, taskStore, _ := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "bad",
		SourcePaths:  `not-json`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: "{}",
		PreHooks:     "[]",
		PostHooks:    "[]",
		ExtraFlags:   "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = taskSvc.RunTask(context.Background(), taskID, "manual")
	if err == nil || !strings.Contains(err.Error(), "parse source paths") {
		t.Fatalf("expected parse source paths error, got %v", err)
	}
}

func TestRunTaskAcceptsFrontendForgetPolicyKeys(t *testing.T) {
	taskSvc, repoStore, taskStore, logPath := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "daily",
		SourcePaths:  `["/data"]`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: `{"keep-last":7}`,
		PreHooks:     "[]",
		PostHooks:    "[]",
		ExtraFlags:   "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := taskSvc.RunTask(context.Background(), taskID, "manual"); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(content)
	if !strings.Contains(args, "forget --keep-last 7") {
		t.Fatalf("expected forget policy to run with --keep-last 7, got %q", args)
	}
}

func TestRunTaskRunsHooksInOrder(t *testing.T) {
	taskSvc, repoStore, taskStore, _ := setupTaskServiceTest(t)
	orderPath := filepath.Join(t.TempDir(), "order.log")
	preHooks, _ := json.Marshal([]string{fmt.Sprintf("printf 'pre\\n' >> %s", strconv.Quote(orderPath))})
	postHooks, _ := json.Marshal([]string{fmt.Sprintf("printf 'post\\n' >> %s", strconv.Quote(orderPath))})

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "hooks",
		SourcePaths:  `["/data"]`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: "{}",
		PreHooks:     string(preHooks),
		PostHooks:    string(postHooks),
		ExtraFlags:   "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := taskSvc.RunTask(context.Background(), taskID, "manual"); err != nil {
		t.Fatal(err)
	}

	logRows, err := taskSvc.repoSvc.store.DB().Query("SELECT command FROM execution_logs WHERE task_id=? ORDER BY id", taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer logRows.Close()
	var commands []string
	for logRows.Next() {
		var command string
		if err := logRows.Scan(&command); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
	}
	if len(commands) != 3 {
		t.Fatalf("expected pre, backup, post logs, got %#v", commands)
	}
	if commands[0] != "pre-hook #1" || !strings.Contains(commands[1], "backup --json /data") || commands[2] != "post-hook #1" {
		t.Fatalf("unexpected command order: %#v", commands)
	}
}

func TestRunTaskDoesNotPersistHookCommandText(t *testing.T) {
	taskSvc, repoStore, taskStore, _ := setupTaskServiceTest(t)
	hookSecret := "super-secret-hook-token"

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	hooks, _ := json.Marshal([]string{fmt.Sprintf("HOOK_TOKEN=%s exit 12", hookSecret)})
	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "secret-hook",
		SourcePaths:  `["/data"]`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: "{}",
		PreHooks:     string(hooks),
		PostHooks:    `[]`,
		ExtraFlags:   "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = taskSvc.RunTask(context.Background(), taskID, "manual")
	if err == nil {
		t.Fatal("expected pre-hook failure")
	}
	if strings.Contains(err.Error(), hookSecret) {
		t.Fatalf("expected hook error to omit raw command text, got %q", err.Error())
	}

	var command string
	if err := taskSvc.repoSvc.store.DB().QueryRow("SELECT command FROM execution_logs WHERE task_id=? ORDER BY id LIMIT 1", taskID).Scan(&command); err != nil {
		t.Fatal(err)
	}
	if command != "pre-hook #1" {
		t.Fatalf("expected sanitized hook command label, got %q", command)
	}
	if strings.Contains(command, hookSecret) {
		t.Fatalf("expected persisted hook command to omit secret, got %q", command)
	}
}

func TestRunTaskAsyncFailsFastWhenRepositoryBusy(t *testing.T) {
	taskSvc, repoStore, taskStore, _ := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "daily",
		SourcePaths:  `["/data"]`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: "{}",
		PreHooks:     `[]`,
		PostHooks:    `[]`,
		ExtraFlags:   `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	reservation, err := taskSvc.repoSvc.acquireRepoOperation(context.Background(), repoID, "sync:core", repoOperationRead, false)
	if err != nil {
		t.Fatal(err)
	}
	defer taskSvc.repoSvc.releaseRepoOperation(reservation)

	logID, err := taskSvc.RunTaskAsync(taskID, "manual")
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("expected repository busy error, got logID=%d err=%v", logID, err)
	}
}

func TestRunTaskStopsWhenPreHookFails(t *testing.T) {
	taskSvc, repoStore, taskStore, logPath := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "pre-fail",
		SourcePaths:  `["/data"]`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: "{}",
		PreHooks:     `["echo nope >&2; exit 12"]`,
		PostHooks:    `[]`,
		ExtraFlags:   "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := taskSvc.RunTask(context.Background(), taskID, "manual")
	if err == nil {
		t.Fatal("expected pre-hook failure")
	}
	if result.ExitCode != 12 {
		t.Fatalf("expected pre-hook exit code 12, got %+v", result)
	}

	content, readErr := os.ReadFile(logPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if strings.TrimSpace(string(content)) != "" {
		t.Fatalf("expected backup command not to run, got %q", content)
	}
}

func TestRunTaskReportsPostHookFailureAfterBackup(t *testing.T) {
	taskSvc, repoStore, taskStore, logPath := setupTaskServiceTest(t)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, taskSvc.repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := taskStore.Create(&model.BackupTask{
		RepoID:       repoID,
		Name:         "post-fail",
		SourcePaths:  `["/data"]`,
		Excludes:     `[]`,
		Tags:         `[]`,
		ForgetPolicy: "{}",
		PreHooks:     `[]`,
		PostHooks:    `["echo cleanup failed >&2; exit 23"]`,
		ExtraFlags:   "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := taskSvc.RunTask(context.Background(), taskID, "manual")
	if err == nil {
		t.Fatal("expected post-hook failure")
	}
	if result.ExitCode != 23 || !strings.Contains(result.Stderr, "cleanup failed") {
		t.Fatalf("expected post-hook failure details, got %+v", result)
	}

	content, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(content), "backup --json /data") {
		t.Fatalf("expected backup to run before post-hook failure, got %q", content)
	}
}

func mustEncryptForTest(t *testing.T, svc *RepoService, plaintext string) string {
	t.Helper()
	ciphertext, err := svc.encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return ciphertext
}
