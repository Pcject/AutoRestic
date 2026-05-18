ALTER TABLE repository_snapshots ADD COLUMN generation INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS repository_sync_state (
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
    PRIMARY KEY (repo_id, domain),
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE,
    FOREIGN KEY (log_id) REFERENCES execution_logs(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS repository_stats (
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
    PRIMARY KEY (repo_id, generation),
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS repository_keys (
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
    PRIMARY KEY (repo_id, generation, key_id),
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS repository_snapshot_files (
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
    PRIMARY KEY (repo_id, snapshot_id, generation, path),
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS repository_snapshot_file_indexes (
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
    PRIMARY KEY (repo_id, snapshot_id, path),
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS repository_restic_config (
    repo_id INTEGER NOT NULL,
    generation INTEGER NOT NULL DEFAULT 0,
    repository_id TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 0,
    raw_json TEXT NOT NULL DEFAULT '{}',
    indexed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (repo_id, generation),
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_repository_snapshots_repo_generation_time
    ON repository_snapshots(repo_id, generation, time DESC);
CREATE INDEX IF NOT EXISTS idx_repository_sync_state_status
    ON repository_sync_state(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_repository_stats_repo_generation
    ON repository_stats(repo_id, generation);
CREATE INDEX IF NOT EXISTS idx_repository_keys_repo_generation
    ON repository_keys(repo_id, generation);
CREATE INDEX IF NOT EXISTS idx_repository_snapshot_files_lookup
    ON repository_snapshot_files(repo_id, snapshot_id, parent_path, generation, name);
CREATE INDEX IF NOT EXISTS idx_repository_snapshot_file_indexes_repo_status
    ON repository_snapshot_file_indexes(repo_id, status, stale, updated_at);
CREATE INDEX IF NOT EXISTS idx_repository_restic_config_repo_generation
    ON repository_restic_config(repo_id, generation);
