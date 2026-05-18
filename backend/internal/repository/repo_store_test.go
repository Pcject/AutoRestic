package repository

import (
	"database/sql"
	"testing"

	"github.com/autorestic/autorestic/internal/model"
	_ "modernc.org/sqlite"
)

func TestRepoStoreUsesEncryptedColumnWhenPresent(t *testing.T) {
	db := openRepoStoreDB(t, true)
	store := NewRepoStore(db)

	id, err := store.Create(&model.Repository{
		Name:                  "encrypted",
		Type:                  "rclone",
		Endpoint:              "bucket/path",
		PasswordEncrypted:     "secret",
		RcloneConfigEncrypted: "ciphertext",
		Options:               "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo, err := store.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.HasRcloneConfig {
		t.Fatal("expected has_rclone_config=true")
	}
	if repo.RcloneConfigEncrypted != "ciphertext" {
		t.Fatalf("expected encrypted config to round-trip, got %q", repo.RcloneConfigEncrypted)
	}
}

func TestRepoStoreFallsBackToLegacyRcloneColumn(t *testing.T) {
	db := openRepoStoreDB(t, false)
	store := NewRepoStore(db)

	_, err := db.Exec(`INSERT INTO repositories (name, type, endpoint, password_encrypted, rclone_config, options)
		VALUES ('legacy', 'rclone', 'bucket/path', 'secret', 'legacy-config', '{}')`)
	if err != nil {
		t.Fatal(err)
	}

	repos, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if !repos[0].HasRcloneConfig {
		t.Fatal("expected legacy rclone config to set has_rclone_config")
	}
	if repos[0].RcloneConfigEncrypted != "legacy-config" {
		t.Fatalf("expected legacy config fallback, got %q", repos[0].RcloneConfigEncrypted)
	}
}

func openRepoStoreDB(t *testing.T, withEncryptedColumn bool) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/repo-store.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
		CREATE TABLE repositories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			password_encrypted TEXT NOT NULL DEFAULT '',
			rclone_config TEXT NOT NULL DEFAULT '',
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
	`
	if withEncryptedColumn {
		schema += `ALTER TABLE repositories ADD COLUMN rclone_config_encrypted TEXT NOT NULL DEFAULT '';`
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}
