package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/repository"
)

func TestParseResticFileEntriesAcceptsLineDelimitedJSON(t *testing.T) {
	raw := `{"name":"/","path":"/","type":"dir","mode":2147484141}
{"name":"notes.txt","path":"/notes.txt","type":"file","size":12,"mode":33188}`

	files, err := parseResticFileEntries(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(files))
	}
	if files[1].Path != "/notes.txt" || files[1].Name != "notes.txt" {
		t.Fatalf("unexpected file entry: %+v", files[1])
	}
}

func TestParseSnapshotsRejectsInvalidJSON(t *testing.T) {
	_, err := parseSnapshots("not-json")
	if err == nil {
		t.Fatal("expected invalid snapshot JSON to return an error")
	}
}

func TestStoreBackupSummaryPreservesMetricsAcrossSnapshotSync(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	if err := os.WriteFile(resticBin, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	createResult, err := repoSvc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	backupOutput := `{"message_type":"status","percent_done":1}
{"message_type":"summary","snapshot_id":"abc123456789","backup_start":"2026-05-01T00:00:00Z","backup_end":"2026-05-01T00:00:10Z","files_new":2,"data_added":4096,"data_added_packed":3072,"total_files_processed":7,"total_bytes_processed":8192}`
	if err := repoSvc.Cache().StoreBackupSummary(createResult.ID, backupOutput); err != nil {
		t.Fatal(err)
	}
	snapshotPayload := `[
	  {"id":"abc123456789","short_id":"abc12345","time":"2026-05-01T00:00:00Z","hostname":"nas","tags":[],"paths":["/data"],"tree":"tree1"}
	]`
	if err := repoSvc.Cache().replaceSnapshots(createResult.ID, 1, snapshotPayload); err != nil {
		t.Fatal(err)
	}

	var totalBytes, dataAddedPacked int64
	var summary string
	if err := db.QueryRow(
		`SELECT total_bytes_processed, data_added_packed, summary
		 FROM repository_snapshots WHERE repo_id=? AND snapshot_id=? AND generation=1`,
		createResult.ID, "abc123456789",
	).Scan(&totalBytes, &dataAddedPacked, &summary); err != nil {
		t.Fatal(err)
	}
	if totalBytes != 8192 || dataAddedPacked != 3072 || !strings.Contains(summary, "total_bytes_processed") {
		t.Fatalf("expected backup summary metrics to survive snapshot sync, got total=%d packed=%d summary=%s", totalBytes, dataAddedPacked, summary)
	}
}

func TestSnapshotIndexSupportsPagination(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "snapshots" ]; then
  cat <<'JSON'
[
  {"id":"aaa111","short_id":"aaa111","time":"2026-05-01T00:00:00Z","hostname":"nas","tags":["daily"],"paths":["/data"],"tree":"tree1"},
  {"id":"bbb222","short_id":"bbb222","time":"2026-05-02T00:00:00Z","hostname":"nas","tags":["weekly"],"paths":["/photos"],"tree":"tree2"},
  {"id":"ccc333","short_id":"ccc333","time":"2026-05-03T00:00:00Z","hostname":"nas","tags":[],"paths":["/code"],"tree":"tree3"}
]
JSON
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	createResult, err := repoSvc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSnapshotService(repoSvc)

	if _, err := repoSvc.Cache().QueueCoreSync(createResult.ID); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		page, err := svc.ListSnapshotIndex(createResult.ID, 1, 2, false)
		if err != nil {
			return false, err
		}
		return page.Total == 3, nil
	})
	page, err := svc.ListSnapshotIndex(createResult.ID, 1, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 2 {
		t.Fatalf("expected first page of indexed snapshots, got total=%d len=%d", page.Total, len(page.Items))
	}
	if page.Items[0].ID != "ccc333" || page.Items[1].ID != "bbb222" {
		t.Fatalf("expected snapshots ordered by newest first, got %#v", page.Items)
	}
}

func TestSnapshotIndexFiltersByUpdateStatus(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, filepath.Join(tmp, "unused-restic"))
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSnapshotService(repoSvc)
	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_sync_state (repo_id, domain, status, generation, last_success_at)
		VALUES (?, 'core', 'success', 1, datetime('now'))
	`, repoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshots
		 (repo_id, snapshot_id, short_id, time, hostname, tags, paths, tree, summary, files_new, files_unmodified, total_files_processed, generation)
		VALUES
		 (?, 'updated', 'updated', '2026-05-03T00:00:00Z', 'nas', '[]', '[]', 'tree', '{"message_type":"summary"}', 1, 0, 1, 1),
		 (?, 'unchanged', 'unchanged', '2026-05-02T00:00:00Z', 'nas', '[]', '[]', 'tree', '{"message_type":"summary"}', 0, 9, 9, 1),
		 (?, 'unknown', 'unknown', '2026-05-01T00:00:00Z', 'nas', '[]', '[]', 'tree', '{}', 0, 0, 0, 1)
	`, repoID, repoID, repoID); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		filter string
		want   string
	}{
		{filter: "updated", want: "updated"},
		{filter: "unchanged", want: "unchanged"},
		{filter: "unknown", want: "unknown"},
	}
	for _, tc := range cases {
		page, err := svc.ListSnapshotIndexFiltered(repoID, 1, 10, false, tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != tc.want {
			t.Fatalf("filter %s expected %s, got total=%d items=%+v", tc.filter, tc.want, page.Total, page.Items)
		}
	}
}

func TestSnapshotIndexStoresResticSnapshotMetadata(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "snapshots" ]; then
  cat <<'JSON'
[
  {
    "id":"aaa111","short_id":"aaa111","time":"2026-05-01T00:00:00Z","hostname":"nas",
    "username":"root","uid":0,"gid":0,"tags":["daily"],"paths":["/data"],"tree":"tree1",
    "program_version":"restic 0.18.1",
    "summary":{
      "backup_start":"2026-05-01T00:00:00Z","backup_end":"2026-05-01T00:01:30Z",
      "files_new":2,"files_changed":3,"files_unmodified":4,
      "dirs_new":5,"dirs_changed":6,"dirs_unmodified":7,
      "data_blobs":8,"tree_blobs":9,"data_added":1024,"data_added_packed":900,
      "total_files_processed":9,"total_bytes_processed":2048
    }
  }
]
JSON
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	createResult, err := repoSvc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSnapshotService(repoSvc)

	if _, err := repoSvc.Cache().QueueCoreSync(createResult.ID); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
		if err != nil {
			return false, err
		}
		return page.Total == 1 && page.Items[0].TotalBytesProcessed == 2048, nil
	})
	page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	snap := page.Items[0]
	if snap.Username != "root" || snap.UID != 0 || snap.GID != 0 || snap.ProgramVersion != "restic 0.18.1" {
		t.Fatalf("snapshot metadata not preserved: %+v", snap)
	}
	if snap.BackupStart == "" || snap.BackupEnd == "" || snap.FilesNew != 2 || snap.FilesChanged != 3 ||
		snap.FilesUnmodified != 4 || snap.DirsNew != 5 || snap.DirsChanged != 6 || snap.DirsUnmodified != 7 ||
		snap.DataBlobs != 8 || snap.TreeBlobs != 9 || snap.DataAdded != 1024 || snap.DataAddedPacked != 900 ||
		snap.TotalFilesProcessed != 9 || snap.TotalBytesProcessed != 2048 {
		t.Fatalf("snapshot summary fields not preserved: %+v", snap)
	}
	if len(snap.Summary) == 0 || !strings.Contains(string(snap.Summary), "total_bytes_processed") {
		t.Fatalf("snapshot raw summary not preserved: %s", string(snap.Summary))
	}
}

func TestSnapshotIndexPreservesStaleStatusOnRefreshFailure(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "snapshots" ]; then
  if [ -f "` + statePath + `" ]; then
    echo "backend offline" >&2
    exit 1
  fi
  cat <<'JSON'
[
  {"id":"snap-1","short_id":"snap-1","time":"2026-05-03T00:00:00Z","hostname":"nas","tags":[],"paths":["/data"],"tree":"tree1"}
]
JSON
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	createResult, err := repoSvc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSnapshotService(repoSvc)

	if _, err := repoSvc.Cache().QueueCoreSync(createResult.ID); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
		if err != nil {
			return false, err
		}
		return page.Total == 1 && !page.Stale, nil
	})
	initialCoreState, _, err := repoSvc.Cache().getSyncState(createResult.ID, syncDomainCore)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("fail"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := repoSvc.Cache().QueueCoreSync(createResult.ID); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
		if err != nil {
			return false, err
		}
		return page.Stale && strings.Contains(page.Error, "backend offline"), nil
	})

	page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Stale || !strings.Contains(page.Error, "backend offline") {
		t.Fatalf("expected stale index error metadata, got %+v", page)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "snap-1" {
		t.Fatalf("expected previously indexed snapshots to remain available, got %+v", page.Items)
	}
	if page.LastIndexed == nil {
		t.Fatalf("expected last indexed timestamp to be preserved, got %+v", page)
	}
	states, err := repoSvc.Cache().GetSyncStates(createResult.ID)
	if err != nil {
		t.Fatal(err)
	}
	var coreState SyncState
	for _, state := range states {
		if state.Domain == syncDomainCore {
			coreState = state
			break
		}
	}
	if coreState.Domain == "" || coreState.Generation != initialCoreState.Generation {
		t.Fatalf("expected failed refresh to preserve previous generation, got %+v", states)
	}
}

func TestSnapshotIndexPreservesPreviousGenerationOnInvalidPayload(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "snapshots" ]; then
  if [ -f "` + statePath + `" ]; then
    echo '{"not":"an array"}'
    exit 0
  fi
  cat <<'JSON'
[
  {"id":"snap-1","short_id":"snap-1","time":"2026-05-03T00:00:00Z","hostname":"nas","tags":[],"paths":["/data"],"tree":"tree1"}
]
JSON
  exit 0
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	createResult, err := repoSvc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSnapshotService(repoSvc)

	if _, err := repoSvc.Cache().QueueCoreSync(createResult.ID); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
		if err != nil {
			return false, err
		}
		return page.Total == 1 && !page.Stale, nil
	})
	initialCoreState, _, err := repoSvc.Cache().getSyncState(createResult.ID, syncDomainCore)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(statePath, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := repoSvc.Cache().QueueCoreSync(createResult.ID); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
		if err != nil {
			return false, err
		}
		return page.Stale && strings.Contains(page.Error, "expected JSON array of snapshots"), nil
	})

	page, err := svc.ListSnapshotIndex(createResult.ID, 1, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "snap-1" {
		t.Fatalf("expected previously indexed snapshots to remain available, got %+v", page.Items)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repository_snapshots WHERE repo_id=? AND generation=?`, createResult.ID, initialCoreState.Generation).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected previous generation %d snapshots to remain intact, count=%d", initialCoreState.Generation, count)
	}
}

func TestListFilesViewReadsDatabaseIndexFirst(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, filepath.Join(tmp, "unused-restic"))
	repoSvc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSnapshotService(repoSvc)

	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, repoSvc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
		INSERT INTO repository_sync_state (repo_id, domain, status, generation, last_success_at)
		VALUES (?, 'core', 'success', 1, datetime('now'))
	`, repoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshots
		 (repo_id, snapshot_id, short_id, time, hostname, tags, paths, tree, raw_json, indexed_at, generation)
		VALUES
		 (?, 'snap-1', 'snap-1', '2026-05-01T00:00:00Z', 'nas', '[]', '["/data"]', 'tree', '{}', datetime('now'), 1)
	`, repoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshot_file_indexes
		 (repo_id, snapshot_id, path, status, entry_count, indexed_at, stale, generation)
		VALUES
		 (?, 'snap-1', '', 'success', 2, datetime('now'), 0, 1)
	`, repoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshot_files
		 (repo_id, snapshot_id, path, parent_path, name, type, size, mode, mtime, raw_json, generation, indexed_at, updated_at)
		VALUES
		 (?, 'snap-1', 'docs', '', 'docs', 'dir', 0, '2147484141', '', '{}', 1, datetime('now'), datetime('now')),
		 (?, 'snap-1', 'notes.txt', '', 'notes.txt', 'file', 12, '33188', '2026-05-01T00:00:00Z', '{}', 1, datetime('now'), datetime('now'))
	`, repoID, repoID); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListFilesView(repoID, "snap-1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if page.Indexing || page.Stale {
		t.Fatalf("expected indexed DB response, got %+v", page)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("expected 2 DB-backed file entries, got %+v", page)
	}
	if page.Items[0].Name != "docs" || page.Items[1].Name != "notes.txt" {
		t.Fatalf("expected DB ordering to be preserved, got %+v", page.Items)
	}
}
