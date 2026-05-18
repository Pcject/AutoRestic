-- backend/migrations/001_init.sql
CREATE TABLE IF NOT EXISTS repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('local', 'rclone', 'webdav')),
    endpoint TEXT NOT NULL,
    password_encrypted TEXT NOT NULL DEFAULT '',
    rclone_config TEXT NOT NULL DEFAULT '',
    rclone_config_encrypted TEXT NOT NULL DEFAULT '',
    webdav_url TEXT NOT NULL DEFAULT '',
    webdav_user TEXT NOT NULL DEFAULT '',
    webdav_password_encrypted TEXT NOT NULL DEFAULT '',
    options TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS execution_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER,
    task_id INTEGER,
    command TEXT NOT NULL,
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    combined_output TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT -1,
    status TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running', 'success', 'failed', 'cancelled')),
    trigger TEXT NOT NULL DEFAULT 'manual' CHECK(trigger IN ('manual', 'scheduled', 'system_query')),
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE SET NULL
);

CREATE INDEX idx_execution_logs_repo_id ON execution_logs(repo_id);
CREATE INDEX idx_execution_logs_status ON execution_logs(status);
CREATE INDEX idx_execution_logs_trigger ON execution_logs(trigger);
CREATE INDEX idx_execution_logs_started_at ON execution_logs(started_at);
