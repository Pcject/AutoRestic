package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenAppliesPragmasToEachConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.sqlite")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.SetMaxOpenConns(2)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		assertPragmaInt(t, conn, "foreign_keys", 1)
		assertPragmaInt(t, conn, "busy_timeout", 5000)
		assertPragmaText(t, conn, "journal_mode", "wal")
	}
}

func TestSQLiteDSNUsesAbsoluteFileURIForRelativePaths(t *testing.T) {
	dsn := sqliteDSN(filepath.Join("data", "autorestic.db"))
	if strings.HasPrefix(dsn, "file://data/") || strings.Contains(dsn, "file://data") {
		t.Fatalf("relative sqlite path produced URI authority: %s", dsn)
	}
	if !strings.HasPrefix(dsn, "file:///") {
		t.Fatalf("expected absolute file URI, got %s", dsn)
	}
}

func TestMigrateRepoMaintenanceIsIdempotentForExistingColumns(t *testing.T) {
	db := openSQLiteForTest(t)

	migrationsDir := t.TempDir()
	writeMigrationFixture(t, migrationsDir, "001_init.sql")
	if err := Migrate(db, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE repositories ADD COLUMN prune_enabled INTEGER NOT NULL DEFAULT 1`); err != nil {
		t.Fatal(err)
	}

	writeMigrationFixture(t, migrationsDir, "003_repo_maintenance.sql")
	if err := Migrate(db, migrationsDir); err != nil {
		t.Fatalf("expected idempotent maintenance migration, got %v", err)
	}

	for _, column := range []string{"prune_enabled", "prune_cron_expr", "prune_args", "check_enabled", "check_cron_expr", "check_args"} {
		if !columnExistsForTest(t, db, "repositories", column) {
			t.Fatalf("expected column %s to exist after migration", column)
		}
	}
}

func TestMigrateRepoUniquenessFailsWithActionableDuplicates(t *testing.T) {
	db := openSQLiteForTest(t)

	migrationsDir := t.TempDir()
	writeMigrationFixture(t, migrationsDir, "001_init.sql")
	if err := Migrate(db, migrationsDir); err != nil {
		t.Fatal(err)
	}

	_, err := db.Exec(`
		INSERT INTO repositories (name, type, endpoint, password_encrypted) VALUES
		('Home', 'local', '/repo-a', 'x'),
		(' home ', 'local', '/repo-b', 'x'),
		('Archive', 'rclone', 'bucket/path', 'x'),
		('Archive 2', 'rclone', 'bucket/path', 'x')
	`)
	if err != nil {
		t.Fatal(err)
	}

	writeMigrationFixture(t, migrationsDir, "004_repo_uniqueness.sql")
	err = Migrate(db, migrationsDir)
	if err == nil {
		t.Fatal("expected duplicate preflight failure")
	}
	msg := err.Error()
	for _, want := range []string{"duplicate repository name", "duplicate repository (rclone, \"bucket/path\")", "before rerunning migrations"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected migration error to contain %q, got %q", want, msg)
		}
	}
}

func TestMigrateRepositoryDBIndexingRebuildsSnapshotsWithGeneration(t *testing.T) {
	db := openSQLiteForTest(t)

	migrationsDir := t.TempDir()
	writeMigrationFixture(t, migrationsDir, "001_init.sql")
	writeMigrationFixture(t, migrationsDir, "006_repository_snapshots.sql")
	if err := Migrate(db, migrationsDir); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
		INSERT INTO repositories (name, type, endpoint, password_encrypted)
		VALUES ('repo', 'local', '/repo', 'secret')
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshots
		 (repo_id, snapshot_id, short_id, time, hostname, tags, paths, tree, raw_json, indexed_at)
		VALUES
		 (1, 'snap-1', 'snap-1', '2026-05-01T00:00:00Z', 'nas', '[]', '[]', 'tree-1', '{}', datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}

	writeMigrationFixture(t, migrationsDir, "008_repository_db_indexing.sql")
	if err := Migrate(db, migrationsDir); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{
		"repository_sync_state",
		"repository_stats",
		"repository_keys",
		"repository_snapshot_files",
		"repository_snapshot_file_indexes",
		"repository_restic_config",
	} {
		if !tableExistsForTest(t, db, table) {
			t.Fatalf("expected table %s to exist after 008 migration", table)
		}
	}
	if !columnExistsForTest(t, db, "repository_snapshots", "generation") {
		t.Fatal("expected repository_snapshots.generation column to exist")
	}

	var generation int
	if err := db.QueryRow(`SELECT generation FROM repository_snapshots WHERE repo_id=1 AND snapshot_id='snap-1'`).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation != 0 {
		t.Fatalf("expected existing snapshots to be preserved at generation 0, got %d", generation)
	}

	var definition string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='repository_snapshots'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(strings.ToLower(definition), " ", ""), "primarykey(repo_id,snapshot_id,generation)") {
		t.Fatalf("expected repository_snapshots primary key to include generation, got %s", definition)
	}
}

func TestMigrateRepositoryDBIndexingClearsZeroSyncLogID(t *testing.T) {
	db := openSQLiteForTest(t)

	migrationsDir := t.TempDir()
	writeMigrationFixture(t, migrationsDir, "001_init.sql")
	writeMigrationFixture(t, migrationsDir, "006_repository_snapshots.sql")
	if err := Migrate(db, migrationsDir); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
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
		)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_sync_state (repo_id, domain, status, log_id)
		VALUES (7, 'core', 'running', 0)
	`); err != nil {
		t.Fatal(err)
	}

	writeMigrationFixture(t, migrationsDir, "008_repository_db_indexing.sql")
	if err := Migrate(db, migrationsDir); err != nil {
		t.Fatal(err)
	}

	var logID sql.NullInt64
	if err := db.QueryRow(`SELECT log_id FROM repository_sync_state WHERE repo_id=7 AND domain='core'`).Scan(&logID); err != nil {
		t.Fatal(err)
	}
	if logID.Valid {
		t.Fatalf("expected migration to clear zero sync log_id, got %d", logID.Int64)
	}
}

func openSQLiteForTest(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func writeMigrationFixture(t *testing.T, dir, name string) {
	t.Helper()
	source := filepath.Join(migrationsRoot(t), name)
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), content, 0644); err != nil {
		t.Fatal(err)
	}
}

func migrationsRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
}

func assertPragmaInt(t *testing.T, conn *sql.Conn, pragma string, want int) {
	t.Helper()
	var got int
	if err := conn.QueryRowContext(context.Background(), "PRAGMA "+pragma).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("expected PRAGMA %s=%d, got %d", pragma, want, got)
	}
}

func assertPragmaText(t *testing.T, conn *sql.Conn, pragma, want string) {
	t.Helper()
	var got string
	if err := conn.QueryRowContext(context.Background(), "PRAGMA "+pragma).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("expected PRAGMA %s=%q, got %q", pragma, want, got)
	}
}

func columnExistsForTest(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
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
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func tableExistsForTest(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		t.Fatal(err)
	}
	return name == table
}
