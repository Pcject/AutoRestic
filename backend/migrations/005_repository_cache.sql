CREATE TABLE IF NOT EXISTS repository_cache (
    repo_id INTEGER NOT NULL,
    cache_key TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '',
    refreshed_at DATETIME NOT NULL DEFAULT (datetime('now')),
    error TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, cache_key),
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_repository_cache_refreshed_at ON repository_cache(refreshed_at);
