-- backend/migrations/002_backup_tasks.sql
CREATE TABLE IF NOT EXISTS backup_tasks (
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
    next_run_at DATETIME,
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE INDEX idx_backup_tasks_repo_id ON backup_tasks(repo_id);
CREATE INDEX idx_backup_tasks_cron_enabled ON backup_tasks(cron_enabled);