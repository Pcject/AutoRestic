package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Migrate(db *sql.DB, migrationsDir string) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", f).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		if err := applyMigration(tx, f, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", f, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", f); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func applyMigration(tx *sql.Tx, version, content string) error {
	switch version {
	case "003_repo_maintenance.sql":
		return applyRepoMaintenanceMigration(tx)
	case "004_repo_uniqueness.sql":
		return applyRepoUniquenessMigration(tx)
	case "007_repository_rclone_config_encrypted.sql":
		return ensureColumn(tx, "repositories", "rclone_config_encrypted", "TEXT NOT NULL DEFAULT ''")
	case "008_repository_db_indexing.sql":
		return applyRepositoryDBIndexingMigration(tx)
	default:
		_, err := tx.Exec(content)
		return err
	}
}

func applyRepoMaintenanceMigration(tx *sql.Tx) error {
	columns := []struct {
		name       string
		definition string
	}{
		{name: "prune_enabled", definition: "INTEGER NOT NULL DEFAULT 1"},
		{name: "prune_cron_expr", definition: "TEXT NOT NULL DEFAULT '0 3 * * 0'"},
		{name: "prune_args", definition: "TEXT NOT NULL DEFAULT '[]'"},
		{name: "check_enabled", definition: "INTEGER NOT NULL DEFAULT 1"},
		{name: "check_cron_expr", definition: "TEXT NOT NULL DEFAULT '0 4 1 * *'"},
		{name: "check_args", definition: "TEXT NOT NULL DEFAULT '[\"--read-data-subset=10%\"]'"},
	}
	for _, column := range columns {
		if err := ensureColumn(tx, "repositories", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func applyRepoUniquenessMigration(tx *sql.Tx) error {
	if err := preflightRepositoryUniqueness(tx); err != nil {
		return err
	}
	_, err := tx.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_repositories_name_unique ON repositories(name);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_repositories_type_endpoint_unique ON repositories(type, endpoint);
	`)
	return err
}

func applyRepositoryDBIndexingMigration(tx *sql.Tx) error {
	if err := ensureRepositorySnapshotsGenerationLayout(tx); err != nil {
		return err
	}
	if err := ensureRepositorySnapshotMetadataColumns(tx); err != nil {
		return err
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS repository_sync_state (
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
		)`,
		`CREATE TABLE IF NOT EXISTS repository_stats (
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
		)`,
		`CREATE TABLE IF NOT EXISTS repository_keys (
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
		)`,
		`CREATE TABLE IF NOT EXISTS repository_snapshot_files (
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
		)`,
		`CREATE TABLE IF NOT EXISTS repository_snapshot_file_indexes (
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
		)`,
		`CREATE TABLE IF NOT EXISTS repository_restic_config (
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_repository_snapshots_repo_generation_time
			ON repository_snapshots(repo_id, generation, time DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_repository_sync_state_status
			ON repository_sync_state(status, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_repository_stats_repo_generation
			ON repository_stats(repo_id, generation)`,
		`CREATE INDEX IF NOT EXISTS idx_repository_keys_repo_generation
			ON repository_keys(repo_id, generation)`,
		`CREATE INDEX IF NOT EXISTS idx_repository_snapshot_files_lookup
			ON repository_snapshot_files(repo_id, snapshot_id, parent_path, generation, name)`,
		`CREATE INDEX IF NOT EXISTS idx_repository_snapshot_file_indexes_repo_status
			ON repository_snapshot_file_indexes(repo_id, status, stale, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_repository_restic_config_repo_generation
			ON repository_restic_config(repo_id, generation)`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE repository_sync_state SET log_id=NULL WHERE log_id IS NOT NULL AND log_id<=0`); err != nil {
		return err
	}
	return nil
}

func ensureRepositorySnapshotsGenerationLayout(tx *sql.Tx) error {
	definition, err := tableDefinition(tx, "repository_snapshots")
	if err != nil {
		return err
	}
	normalized := strings.ReplaceAll(strings.ToLower(definition), " ", "")
	if strings.Contains(normalized, "primarykey(repo_id,snapshot_id,generation)") {
		if exists, err := columnExists(tx, "repository_snapshots", "generation"); err != nil {
			return err
		} else if exists {
			return nil
		}
	}

	hasGeneration, err := columnExists(tx, "repository_snapshots", "generation")
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS repository_snapshots_new (
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
		)
	`); err != nil {
		return err
	}

	selectGeneration := "0"
	if hasGeneration {
		selectGeneration = "generation"
	}
	if _, err := tx.Exec(`
		INSERT INTO repository_snapshots_new
		 (repo_id, snapshot_id, short_id, time, hostname, tags, paths, tree, raw_json, indexed_at, generation)
		 SELECT repo_id, snapshot_id, short_id, time, hostname, tags, paths, tree, raw_json, indexed_at, ` + selectGeneration + `
		 FROM repository_snapshots
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE repository_snapshots`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE repository_snapshots_new RENAME TO repository_snapshots`); err != nil {
		return err
	}
	return nil
}

func ensureRepositorySnapshotMetadataColumns(tx *sql.Tx) error {
	columns := []struct {
		name       string
		definition string
	}{
		{name: "username", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "uid", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "gid", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "program_version", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "summary", definition: "TEXT NOT NULL DEFAULT '{}'"},
		{name: "backup_start", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "backup_end", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "files_new", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "files_changed", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "files_unmodified", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "dirs_new", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "dirs_changed", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "dirs_unmodified", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "data_blobs", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "tree_blobs", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "data_added", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "data_added_packed", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "total_files_processed", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "total_bytes_processed", definition: "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		if err := ensureColumn(tx, "repository_snapshots", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func tableDefinition(tx *sql.Tx, table string) (string, error) {
	var definition string
	err := tx.QueryRow(`SELECT COALESCE(sql, '') FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&definition)
	return definition, err
}

func ensureColumn(tx *sql.Tx, table, column, definition string) error {
	exists, err := columnExists(tx, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func preflightRepositoryUniqueness(tx *sql.Tx) error {
	var problems []string

	nameRows, err := tx.Query(`
		SELECT TRIM(name) AS normalized_name, GROUP_CONCAT(id)
		FROM repositories
		GROUP BY LOWER(TRIM(name))
		HAVING COUNT(*) > 1
		ORDER BY LOWER(TRIM(name))
	`)
	if err != nil {
		return err
	}
	defer nameRows.Close()

	for nameRows.Next() {
		var name string
		var ids string
		if err := nameRows.Scan(&name, &ids); err != nil {
			return err
		}
		problems = append(problems, fmt.Sprintf("duplicate repository name %q for ids [%s]", name, ids))
	}
	if err := nameRows.Err(); err != nil {
		return err
	}

	endpointRows, err := tx.Query(`
		SELECT type, TRIM(endpoint) AS normalized_endpoint, GROUP_CONCAT(id)
		FROM repositories
		GROUP BY type, TRIM(endpoint)
		HAVING COUNT(*) > 1
		ORDER BY type, TRIM(endpoint)
	`)
	if err != nil {
		return err
	}
	defer endpointRows.Close()

	for endpointRows.Next() {
		var repoType string
		var endpoint string
		var ids string
		if err := endpointRows.Scan(&repoType, &endpoint, &ids); err != nil {
			return err
		}
		problems = append(problems, fmt.Sprintf("duplicate repository (%s, %q) for ids [%s]", repoType, endpoint, ids))
	}
	if err := endpointRows.Err(); err != nil {
		return err
	}

	if len(problems) == 0 {
		return nil
	}

	return errors.New("repository uniqueness preflight failed: resolve " + strings.Join(problems, "; ") + " before rerunning migrations")
}
