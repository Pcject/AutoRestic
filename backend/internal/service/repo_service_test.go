package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/repository"
	_ "modernc.org/sqlite"
)

func TestCreateWebDAVRepoUsesWebDAVURLAsEndpointFallback(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "restic")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	createResult, err := svc.Create(model.CreateRepoRequest{
		Name:      "webdav",
		Type:      "webdav",
		Password:  "secret",
		WebdavURL: "https://example.com/dav",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repoStore.GetByID(createResult.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Endpoint != "https://example.com/dav" {
		t.Fatalf("expected endpoint fallback to webdav url, got %q", repo.Endpoint)
	}
}

func TestCreateRepoDefaultsMaintenanceSchedules(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "restic")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	createResult, err := svc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo, err := repoStore.GetByID(createResult.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.PruneEnabled || repo.PruneCronExpr != "0 3 * * 0" || repo.PruneArgs != "[]" {
		t.Fatalf("unexpected prune defaults: enabled=%v cron=%q args=%q", repo.PruneEnabled, repo.PruneCronExpr, repo.PruneArgs)
	}
	if !repo.CheckEnabled || repo.CheckCronExpr != "0 4 1 * *" || repo.CheckArgs != `["--read-data-subset=10%"]` {
		t.Fatalf("unexpected check defaults: enabled=%v cron=%q args=%q", repo.CheckEnabled, repo.CheckCronExpr, repo.CheckArgs)
	}
}

func TestMaintenanceCommandsUseStoredArgs(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "restic-args.log")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	createResult, err := svc.Create(model.CreateRepoRequest{
		Name:      "local",
		Type:      "local",
		Endpoint:  "/repo",
		Password:  "secret",
		CheckArgs: `["--read-data-subset=10%"]`,
		PruneArgs: `["--max-unused","5%"]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CheckRepo(context.Background(), createResult.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PruneRepo(context.Background(), createResult.ID); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(content)
	if !strings.Contains(args, "check --read-data-subset=10%") {
		t.Fatalf("expected check args to be used, got %q", args)
	}
	if !strings.Contains(args, "prune --max-unused 5%") {
		t.Fatalf("expected prune args to be used, got %q", args)
	}
}

func TestCreateExistingRepoRejectsWrongPassword(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  if [ "$RESTIC_PASSWORD" = "correct-password" ]; then
    exit 0
  fi
  echo "wrong password or no key found" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create(model.CreateRepoRequest{
		Name:     "existing",
		Type:     "local",
		Endpoint: "/repo",
		Password: "wrong-password",
	})
	if !errors.Is(err, ErrRepositoryAccessInvalid) {
		t.Fatalf("expected repository access validation error, got %v", err)
	}

	repos, err := repoStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected wrong-password repo not to be saved, got %d repos", len(repos))
	}
}

func TestCreateExistingRepoRejectsForbiddenAccess(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo "Fatal: unable to open config file: Stat: 403 Forbidden" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create(model.CreateRepoRequest{
		Name:     "forbidden",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if !errors.Is(err, ErrRepositoryAccessInvalid) {
		t.Fatalf("expected repository access validation error, got %v", err)
	}

	repos, err := repoStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected forbidden repo not to be saved, got %d repos", len(repos))
	}
}

func TestCreateLocalRepoRejectsResticLayoutWhenProbeIsInconclusive(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	repoPath := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoPath, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"index", "keys"} {
		if err := os.MkdirAll(filepath.Join(repoPath, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repoPath, "config"), []byte(`{"version":2}`), 0600); err != nil {
		t.Fatal(err)
	}

	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo "transient backend failure" >&2
  exit 1
fi
if [ "$1" = "init" ]; then
  echo "init should not be called" >&2
  exit 2
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	probe, err := svc.CheckRepoPath(context.Background(), model.RepositoryAccessRequest{
		Type:     "local",
		Endpoint: repoPath,
		Password: "wrong-password",
	})
	if !errors.Is(err, ErrRepositoryAccessInvalid) {
		t.Fatalf("expected restic-looking path to be treated as inaccessible existing repo, got %v", err)
	}
	if !probe.Exists || probe.Accessible {
		t.Fatalf("unexpected probe result: %+v", probe)
	}

	_, err = svc.Create(model.CreateRepoRequest{
		Name:         "existing-layout",
		Type:         "local",
		Endpoint:     repoPath,
		Password:     "wrong-password",
		InitOnCreate: true,
	})
	if !errors.Is(err, ErrRepositoryAccessInvalid) {
		t.Fatalf("expected create to reject restic-looking path, got %v", err)
	}

	repos, err := repoStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected inaccessible existing repo not to be saved, got %d repos", len(repos))
	}

	var initCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM execution_logs WHERE command LIKE '% init'`).Scan(&initCount); err != nil {
		t.Fatal(err)
	}
	if initCount != 0 {
		t.Fatalf("expected init not to be called, got %d init logs", initCount)
	}
}

func TestCreateRepoRejectsDuplicateTypeEndpoint(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "true")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create(model.CreateRepoRequest{
		Name:     "one",
		Type:     "local",
		Endpoint: " /repo ",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create(model.CreateRepoRequest{
		Name:     "two",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if !errors.Is(err, ErrRepositoryAlreadyExists) {
		t.Fatalf("expected duplicate repository error, got %v", err)
	}
}

func TestCreateRepoRejectsDuplicateName(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "true")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create(model.CreateRepoRequest{
		Name:     "ProjectVault",
		Type:     "local",
		Endpoint: "/repo-one",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Create(model.CreateRepoRequest{
		Name:     "projectvault",
		Type:     "local",
		Endpoint: "/repo-two",
		Password: "secret",
	})
	if !errors.Is(err, ErrRepositoryAlreadyExists) {
		t.Fatalf("expected duplicate repository name error, got %v", err)
	}
}

func TestGetStatsUsesCacheUntilRefresh(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	countPath := filepath.Join(tmp, "stats-count")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo "repository does not exist" >&2
  exit 1
fi
if [ "$1" = "stats" ]; then
  count=0
  if [ -f "` + countPath + `" ]; then
    count=$(cat "` + countPath + `")
  fi
  count=$((count + 1))
  printf '%s' "$count" > "` + countPath + `"
  printf '{"total_size":%s}\n' "$count"
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	createResult, err := svc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.GetStatsCached(context.Background(), createResult.ID, true); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		view, err := svc.Cache().GetStatsView(createResult.ID, false)
		if err != nil {
			return false, err
		}
		return strings.Contains(string(view.Data), `"total_size":1`), nil
	})
	first, err := svc.GetStatsCached(context.Background(), createResult.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.GetStatsCached(context.Background(), createResult.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Stdout != second.Stdout || !strings.Contains(second.Stdout, `"total_size":1`) {
		t.Fatalf("expected second read to use cached stats, first=%q second=%q", first.Stdout, second.Stdout)
	}

	if _, err := svc.GetStatsCached(context.Background(), createResult.ID, true); err != nil {
		t.Fatal(err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		view, err := svc.Cache().GetStatsView(createResult.ID, false)
		if err != nil {
			return false, err
		}
		return strings.Contains(string(view.Data), `"total_size":2`), nil
	})
	refreshed, err := svc.GetStatsCached(context.Background(), createResult.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(refreshed.Stdout, `"total_size":2`) {
		t.Fatalf("expected forced refresh to rerun stats, got %q", refreshed.Stdout)
	}
}

func TestConcurrentStatsLoadRunsResticOnce(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	countPath := filepath.Join(tmp, "stats-count")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo "repository does not exist" >&2
  exit 1
fi
if [ "$1" = "stats" ]; then
  sleep 1
  count=0
  if [ -f "` + countPath + `" ]; then
    count=$(cat "` + countPath + `")
  fi
  count=$((count + 1))
  printf '%s' "$count" > "` + countPath + `"
  printf '{"total_size":%s}\n' "$count"
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	createResult, err := svc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	logIDs := make([]int64, 2)
	errs := make([]error, 2)
	for i := range logIDs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logIDs[i], errs[i] = svc.Cache().QueueStatsSync(createResult.ID)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if logIDs[0] == 0 || logIDs[1] == 0 || logIDs[0] != logIDs[1] {
		t.Fatalf("expected concurrent refresh requests to dedupe to one log id, got %#v", logIDs)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		view, err := svc.Cache().GetStatsView(createResult.ID, false)
		if err != nil {
			return false, err
		}
		return strings.Contains(string(view.Data), `"total_size":1`), nil
	})
	content, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != "1" {
		t.Fatalf("expected restic stats to run once, count=%q", content)
	}
}

func TestLoadOrCreateKeyRejectsInvalidExistingKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "enc.key")
	if err := os.WriteFile(keyPath, []byte("not-a-valid-key"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := loadOrCreateKey(keyPath)
	if err == nil {
		t.Fatal("expected invalid key file to fail")
	}

	content, readErr := os.ReadFile(keyPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "not-a-valid-key" {
		t.Fatalf("expected existing invalid key to be preserved, got %q", content)
	}
}

func TestRunResticCommandWritesTemporaryRcloneConfig(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "rclone.log")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
printf 'repo=%s\n' "$RESTIC_REPOSITORY" >> "` + logPath + `"
printf 'cfg=%s\n' "$RCLONE_CONFIG" >> "` + logPath + `"
if [ -n "$RCLONE_CONFIG" ]; then
  stat -f 'mode=%Lp' "$RCLONE_CONFIG" >> "` + logPath + `"
  cat "$RCLONE_CONFIG" >> "` + logPath + `"
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	repoID, err := repoStore.Create(&model.Repository{
		Name:                  "remote",
		Type:                  "rclone",
		Endpoint:              "remote:repo",
		PasswordEncrypted:     mustEncryptForTest(t, svc, "secret"),
		RcloneConfigEncrypted: mustEncryptForTest(t, svc, "[remote]\ntype = local\nnounc = true\n"),
		Options:               "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RunResticCommand(context.Background(), repoID, "manual", []string{"snapshots", "--json"}, nil); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(content)
	if !strings.Contains(logText, "repo=rclone:remote:repo") {
		t.Fatalf("expected rclone repo env, got %q", logText)
	}
	if !strings.Contains(logText, "mode=600") {
		t.Fatalf("expected temp rclone config mode 600, got %q", logText)
	}
	if !strings.Contains(logText, "type = local") {
		t.Fatalf("expected temp rclone config contents, got %q", logText)
	}

	var configPath string
	for _, line := range strings.Split(logText, "\n") {
		if strings.HasPrefix(line, "cfg=") {
			configPath = strings.TrimPrefix(line, "cfg=")
		}
	}
	if configPath == "" {
		t.Fatalf("expected logged config path, got %q", logText)
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temp rclone config to be removed, stat err=%v", err)
	}
}

func TestGetStatsCachedReturnsCachedError(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	countPath := filepath.Join(tmp, "stats-count")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo "repository does not exist" >&2
  exit 1
fi
if [ "$1" = "stats" ]; then
  count=0
  if [ -f "` + countPath + `" ]; then
    count=$(cat "` + countPath + `")
  fi
  count=$((count + 1))
  printf '%s' "$count" > "` + countPath + `"
  echo '{"total_size":1}'
  echo 'repository temporarily unavailable' >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	createResult, err := svc.Create(model.CreateRepoRequest{
		Name:     "local",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.GetStatsCached(context.Background(), createResult.ID, true); err != nil {
		t.Fatalf("expected refresh request to queue background work, got %v", err)
	}
	waitForTestCondition(t, 5*time.Second, func() (bool, error) {
		view, err := svc.Cache().GetStatsView(createResult.ID, false)
		if err != nil {
			return false, err
		}
		return view.Stale && strings.Contains(view.Error, "repository temporarily unavailable"), nil
	})

	second, err := svc.GetStatsCached(context.Background(), createResult.ID, false)
	if err == nil {
		t.Fatal("expected cached error to be returned")
	}
	if second.ExitCode == 0 || !strings.Contains(second.Stderr, "repository temporarily unavailable") {
		t.Fatalf("expected cached failure metadata in second result, got %+v", second)
	}

	content, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != strconv.Itoa(1) {
		t.Fatalf("expected cached error to avoid rerun, count=%q", content)
	}
}

func TestCreateExistingRepoStartsAsyncImport(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "snapshots" ]; then
  echo '[]'
  exit 0
fi
if [ "$1" = "stats" ]; then
  echo 'stats should not run during initial import' >&2
  exit 99
fi
if [ "$1" = "key" ] && [ "$2" = "list" ]; then
  echo 'key list should not run during initial import' >&2
  exit 99
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.Create(model.CreateRepoRequest{
		Name:     "existing",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == 0 || result.ImportStatus != syncStatusRunning || result.ImportLogID == 0 {
		t.Fatalf("expected async import metadata for existing repo, got %+v", result)
	}
	waitForTestCondition(t, time.Second, func() (bool, error) {
		state, ok, err := svc.Cache().getSyncState(result.ID, syncDomainCore)
		if err != nil {
			return false, err
		}
		return ok && state.Status == syncStatusSuccess, nil
	})

	var heavyLogs int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM execution_logs
		 WHERE repo_id=? AND (command LIKE '%stats --json%' OR command LIKE '%key list%')`,
		result.ID,
	).Scan(&heavyLogs); err != nil {
		t.Fatal(err)
	}
	if heavyLogs != 0 {
		t.Fatalf("expected initial import to avoid stats/key commands, got %d logs", heavyLogs)
	}
	statsState, ok, err := svc.Cache().getSyncState(result.ID, syncDomainStats)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || statsState.Status != syncStatusStale || !strings.Contains(statsState.LastError, "on demand") {
		t.Fatalf("expected stats to be stale/on-demand after initial import, got ok=%v state=%+v", ok, statsState)
	}
}

func TestProbeRepositoryAccessDetectsExistingLocks(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "list" ] && [ "$2" = "locks" ]; then
  echo '1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef'
  exit 0
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	probe, err := svc.ProbeRepositoryAccess(context.Background(), model.RepositoryAccessRequest{
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !probe.Exists || !probe.Accessible || !probe.Locked {
		t.Fatalf("expected accessible locked repository, got %+v", probe)
	}
}

func TestCreateExistingLockedRepoDoesNotAutoUnlockWithoutPermission(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	lockStatePath := filepath.Join(tmp, "repo.locked")
	unlockLogPath := filepath.Join(tmp, "unlock.log")
	if err := os.WriteFile(lockStatePath, []byte("locked"), 0600); err != nil {
		t.Fatal(err)
	}

	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "list" ] && [ "$2" = "locks" ]; then
  if [ -f "` + lockStatePath + `" ]; then
    echo '1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef'
  fi
  exit 0
fi
if [ "$1" = "unlock" ]; then
  echo 'unlock should not run' >> "` + unlockLogPath + `"
  rm -f "` + lockStatePath + `"
  exit 0
fi
if [ "$1" = "snapshots" ] && [ "$2" = "--json" ]; then
  echo '[]'
  exit 0
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.Create(model.CreateRepoRequest{
		Name:     "existing-locked",
		Type:     "local",
		Endpoint: "/repo",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("expected create to continue without unlock, got %v", err)
	}
	if _, err := os.Stat(unlockLogPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected unlock not to run, stat err=%v", err)
	}
	if result.ID == 0 || result.UnlockLogID != 0 || result.ImportStatus != syncStatusRunning || result.ImportLogID == 0 {
		t.Fatalf("expected import to start without unlock, got %+v", result)
	}
}

func TestCreateExistingLockedRepoAutoUnlocksWhenRequested(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	lockStatePath := filepath.Join(tmp, "repo.locked")
	unlockLogPath := filepath.Join(tmp, "unlock.log")
	if err := os.WriteFile(lockStatePath, []byte("locked"), 0600); err != nil {
		t.Fatal(err)
	}

	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
if [ "$1" = "cat" ] && [ "$2" = "config" ]; then
  echo '{}'
  exit 0
fi
if [ "$1" = "list" ] && [ "$2" = "locks" ]; then
  if [ -f "` + lockStatePath + `" ]; then
    echo '1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef'
  fi
  exit 0
fi
if [ "$1" = "unlock" ]; then
  echo "$*" >> "` + unlockLogPath + `"
  rm -f "` + lockStatePath + `"
  exit 0
fi
if [ "$1" = "snapshots" ] && [ "$2" = "--json" ]; then
  echo '[]'
  exit 0
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.Create(model.CreateRepoRequest{
		Name:       "existing-locked",
		Type:       "local",
		Endpoint:   "/repo",
		Password:   "secret",
		AutoUnlock: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UnlockLogID == 0 {
		t.Fatalf("expected unlock log id when auto unlock is allowed, got %+v", result)
	}
	waitForTestCondition(t, time.Second, func() (bool, error) {
		state, ok, err := svc.Cache().getSyncState(result.ID, syncDomainCore)
		if err != nil {
			return false, err
		}
		return ok && state.Status == syncStatusSuccess, nil
	})
	content, err := os.ReadFile(unlockLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "unlock") {
		t.Fatalf("expected unlock command to run, got %q", content)
	}
	if _, err := os.Stat(lockStatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected lock state to be cleared, stat err=%v", err)
	}
}

func TestRepoCacheStartupPreservesRunningState(t *testing.T) {
	db := setupRepoServiceDB(t)
	if _, err := db.Exec(`
		INSERT INTO repository_sync_state (repo_id, domain, status, phase, progress, generation)
		VALUES (99, 'core', 'running', 'snapshots', 50, 2)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshot_file_indexes (repo_id, snapshot_id, path, status, stale, generation)
		VALUES (99, 'snap-1', '', 'running', 0, 3)
	`); err != nil {
		t.Fatal(err)
	}

	cache := NewRepoCacheService(db, nil)
	defer cache.Stop()

	var syncStatus string
	if err := db.QueryRow(`SELECT status FROM repository_sync_state WHERE repo_id=99 AND domain='core'`).Scan(&syncStatus); err != nil {
		t.Fatal(err)
	}
	if syncStatus != syncStatusRunning {
		t.Fatalf("expected running sync to be preserved, got %q", syncStatus)
	}

	var fileStatus string
	var stale int
	if err := db.QueryRow(`SELECT status, stale FROM repository_snapshot_file_indexes WHERE repo_id=99 AND snapshot_id='snap-1' AND path=''`).Scan(&fileStatus, &stale); err != nil {
		t.Fatal(err)
	}
	if fileStatus != syncStatusRunning || stale != 0 {
		t.Fatalf("expected running file index to be preserved, got status=%q stale=%d", fileStatus, stale)
	}
}

func TestRepoCachePlannerSkipsReposWithoutSyncStateOnStart(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)

	tmp := t.TempDir()
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	if err := os.WriteFile(resticBin, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}

	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repoStore.Create(&model.Repository{
		Name:              "cold-start",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, svc, "secret"),
		Options:           "{}",
	}); err != nil {
		t.Fatal(err)
	}

	svc.Cache().StartPlanner()
	defer svc.Cache().Stop()
	time.Sleep(150 * time.Millisecond)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM execution_logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected planner startup to skip repos without sync state, got %d execution logs", count)
	}
}

func TestRepoCachePlannerDoesNotReplayRunningStateOnStart(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)

	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "key")
	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
case "$*" in
  "cat config")
    printf '{"version":2,"id":"repo-id"}\n'
    ;;
  "snapshots --json")
    printf '[]\n'
    ;;
  *)
    printf 'unexpected restic args: %s\n' "$*" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, keyPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	repoID, err := repoStore.Create(&model.Repository{
		Name:              "interrupted-start",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: mustEncryptForTest(t, svc, "secret"),
		Options:           "{}",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO repository_sync_state
		 (repo_id, domain, status, phase, progress, generation, last_success_at, updated_at)
		VALUES (?, 'core', 'running', 'snapshots', 50, 1, ?, ?)
	`, repoID, now.Add(-time.Hour), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	restartedSvc, err := NewRepoService(repoStore, exec, keyPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	restartedSvc.Cache().StartPlanner()
	defer restartedSvc.Cache().Stop()

	time.Sleep(150 * time.Millisecond)

	var status string
	if err := db.QueryRow(`SELECT status FROM repository_sync_state WHERE repo_id=? AND domain='core'`, repoID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != syncStatusRunning {
		t.Fatalf("expected running startup state to remain running, got %q", status)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM execution_logs WHERE repo_id=?`, repoID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected planner not to replay a running sync state, got %d logs", count)
	}
}

func TestPlanRefreshRoundSkipsMissingAndSuccessStates(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "restic")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	cache := svc.Cache()
	defer cache.Stop()

	mustCreateRepo := func(name string) int64 {
		id, err := repoStore.Create(&model.Repository{
			Name:              name,
			Type:              "local",
			Endpoint:          "/" + name,
			PasswordEncrypted: mustEncryptForTest(t, svc, "secret"),
			Options:           "{}",
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	missingID := mustCreateRepo("missing")
	staleID := mustCreateRepo("stale")
	successID := mustCreateRepo("success")
	now := time.Now().UTC()

	if _, err := db.Exec(`
		INSERT INTO repository_sync_state
		 (repo_id, domain, status, generation, last_success_at, updated_at)
		VALUES
		 (?, 'core', 'stale', 2, ?, ?),
		 (?, 'core', 'success', 3, ?, ?)
	`, staleID, now.Add(-24*time.Hour), now.Add(-2*time.Hour), successID, now.Add(-48*time.Hour), now.Add(-90*time.Minute)); err != nil {
		t.Fatal(err)
	}

	repos, err := repoStore.List()
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := cache.planRefreshRound(context.Background(), repos, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected only stale candidates, got %+v", candidates)
	}
	if candidates[0].repoID != staleID || candidates[0].domain != syncDomainCore || candidates[0].priority != 0 {
		t.Fatalf("unexpected first candidate: %+v", candidates[0])
	}
	for _, candidate := range candidates {
		if candidate.repoID == missingID {
			t.Fatalf("repo without sync state should not be auto-queued: %+v", candidates)
		}
		if candidate.repoID == successID {
			t.Fatalf("successful sync state should not be auto-refreshed: %+v", candidates)
		}
	}
}

func TestPlanRefreshRoundLimitsQueuedRepos(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "restic")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	cache := svc.Cache()
	defer cache.Stop()

	now := time.Now().UTC()
	for i := 0; i < maxPlannerQueuesPerRound+2; i++ {
		repoID, err := repoStore.Create(&model.Repository{
			Name:              "repo-" + strconv.Itoa(i),
			Type:              "local",
			Endpoint:          "/repo-" + strconv.Itoa(i),
			PasswordEncrypted: mustEncryptForTest(t, svc, "secret"),
			Options:           "{}",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
			INSERT INTO repository_sync_state
			 (repo_id, domain, status, generation, last_success_at, updated_at)
			VALUES (?, 'core', 'stale', 1, ?, ?)
		`, repoID, now.Add(-24*time.Hour), now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	repos, err := repoStore.List()
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := cache.planRefreshRound(context.Background(), repos, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != maxPlannerQueuesPerRound {
		t.Fatalf("expected refresh round to cap at %d, got %d", maxPlannerQueuesPerRound, len(candidates))
	}
}

func TestGetSyncStateDomainsBuildsDomainMapAndAggregate(t *testing.T) {
	db := setupRepoServiceDB(t)
	cache := NewRepoCacheService(db, nil)
	defer cache.Stop()

	now := time.Now().UTC()
	earlier := now.Add(-2 * time.Minute)
	later := now.Add(-time.Minute)
	if _, err := db.Exec(`
		INSERT INTO repository_sync_state
		 (repo_id, domain, status, phase, progress, generation, last_success_at, last_error, log_id, updated_at)
		VALUES
		 (42, 'core', 'success', '', 100, 3, ?, '', 101, ?),
		 (42, 'files', 'running', 'snap-1:/', 35, 2, ?, '', 202, ?),
		 (42, 'stats', 'stale', '', 0, 1, ?, 'waiting for refresh', NULL, ?)
	`, earlier, earlier, later, later, earlier, now); err != nil {
		t.Fatal(err)
	}

	domains, err := cache.GetSyncStateDomains(42)
	if err != nil {
		t.Fatal(err)
	}
	if domains["core"].Status != syncStatusSuccess || domains["core"].Generation != 3 {
		t.Fatalf("unexpected core domain state: %+v", domains["core"])
	}
	if domains["files"].Status != syncStatusRunning || domains["files"].LogID == nil || *domains["files"].LogID != 202 {
		t.Fatalf("unexpected files domain state: %+v", domains["files"])
	}
	all, ok := domains["all"]
	if !ok {
		t.Fatalf("expected aggregate all domain in response, got %+v", domains)
	}
	if all.Status != syncStatusRunning {
		t.Fatalf("expected aggregate status to report running while files sync is active, got %+v", all)
	}
	if all.LastSyncedAt == nil || !all.LastSyncedAt.Equal(earlier) {
		t.Fatalf("expected aggregate to preserve latest success time, got %+v", all)
	}
	if all.LogID == nil || *all.LogID != 202 {
		t.Fatalf("expected aggregate to surface active log id, got %+v", all)
	}
}

func TestListSnapshotIndexWithoutSyncStateIsStale(t *testing.T) {
	db := setupRepoServiceDB(t)
	cache := NewRepoCacheService(db, nil)
	defer cache.Stop()

	page, err := cache.ListSnapshotIndex(42, 1, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Stale || page.SyncStatus != syncStatusStale {
		t.Fatalf("expected missing core sync state to be stale, got %+v", page)
	}
}

func TestRootFileIndexUsesSnapshotPathsWithoutResticScan(t *testing.T) {
	db := setupRepoServiceDB(t)
	cache := NewRepoCacheService(db, nil)
	defer cache.Stop()

	if _, err := db.Exec(`
		INSERT INTO repository_sync_state (repo_id, domain, status, generation, last_success_at)
		VALUES (42, 'core', 'success', 1, datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshots
		 (repo_id, snapshot_id, short_id, time, hostname, tags, paths, tree, raw_json, indexed_at, generation)
		VALUES
		 (42, 'snap-1', 'snap-1', '2026-05-01T00:00:00Z', 'nas', '[]', '["/data/projects","/var/log"]', 'tree', '{}', datetime('now'), 1)
	`); err != nil {
		t.Fatal(err)
	}

	page, err := cache.ListSnapshotFiles(42, "snap-1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if page.Indexing || page.Stale {
		t.Fatalf("expected root file index from snapshot paths to be immediately usable, got %+v", page)
	}
	if len(page.Items) != 2 || page.Items[0].Path != "/data" || page.Items[1].Path != "/var" {
		t.Fatalf("expected top-level roots from snapshot paths, got %+v", page.Items)
	}
}

func TestQueueFilesSyncMarksPrewarmAsStalePhase(t *testing.T) {
	db := setupRepoServiceDB(t)
	cache := NewRepoCacheService(db, nil)
	defer cache.Stop()

	if _, err := db.Exec(`
		INSERT INTO repository_sync_state (repo_id, domain, status, generation, last_success_at, updated_at)
		VALUES (42, 'core', 'success', 1, datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repository_snapshots
		 (repo_id, snapshot_id, short_id, time, hostname, tags, paths, tree, raw_json, indexed_at, generation)
		VALUES
		 (42, 'snap-1', 'snap-1', '2026-05-01T00:00:00Z', 'nas', '[]', '["/data/projects","/var/log"]', 'tree', '{}', datetime('now'), 1)
	`); err != nil {
		t.Fatal(err)
	}

	if _, err := cache.QueueFilesSync(42); err != nil {
		t.Fatal(err)
	}

	waitForTestCondition(t, time.Second, func() (bool, error) {
		state, ok, err := cache.getSyncState(42, syncDomainFiles)
		if err != nil {
			return false, err
		}
		return ok && state.Status == syncStatusStale && state.Phase == filesPhaseRootsPrewarmed, nil
	})

	state, ok, err := cache.getSyncState(42, syncDomainFiles)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected files sync state to exist after prewarm")
	}
	if state.Status != syncStatusStale || state.Phase != filesPhaseRootsPrewarmed {
		t.Fatalf("expected prewarm to remain stale with explicit phase, got %+v", state)
	}
	if state.LastSuccessAt == nil {
		t.Fatalf("expected prewarm to record last success time, got %+v", state)
	}
	if state.LogID != nil {
		t.Fatalf("expected prewarm without restic command to keep log_id nil, got %+v", state)
	}
}

func TestSetSyncStateSanitizesZeroLogID(t *testing.T) {
	db := setupRepoServiceDB(t)
	cache := NewRepoCacheService(db, nil)
	defer cache.Stop()

	var zero int64
	if err := cache.setSyncState(42, syncDomainCore, syncStatusRunning, "snapshots", 50, 0, nil, "", &zero); err != nil {
		t.Fatal(err)
	}

	var logID sql.NullInt64
	if err := db.QueryRow(`SELECT log_id FROM repository_sync_state WHERE repo_id=42 AND domain='core'`).Scan(&logID); err != nil {
		t.Fatal(err)
	}
	if logID.Valid {
		t.Fatalf("expected zero log_id to be stored as NULL, got %d", logID.Int64)
	}
}

func TestAcquireRepoOperationSerializesPerRepository(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "restic")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}

	first, err := svc.acquireRepoOperation(context.Background(), 42, "backup", repoOperationExclusive, false)
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan struct{})
	go func() {
		second, err := svc.acquireRepoOperation(context.Background(), 42, "sync:stats", repoOperationRead, true)
		if err != nil {
			t.Errorf("unexpected acquire error: %v", err)
			return
		}
		defer svc.releaseRepoOperation(second)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("second repo operation should wait for the first one to release")
	case <-time.After(120 * time.Millisecond):
	}

	svc.releaseRepoOperation(first)

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second repo operation did not acquire after release")
	}
}

func TestScheduledCheckUnlocksForeignResticLockBeforeRun(t *testing.T) {
	db := setupRepoServiceDB(t)
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "foreign.lock")
	logPath := filepath.Join(tmp, "restic.log")
	if err := os.WriteFile(lockPath, []byte("locked"), 0600); err != nil {
		t.Fatal(err)
	}

	resticBin := filepath.Join(tmp, "restic-stub.sh")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + logPath + `"
if [ "$1" = "list" ] && [ "$2" = "locks" ]; then
  if [ -f "` + lockPath + `" ]; then
    echo '[{"hostname":"other-host","pid":999999,"exclusive":true}]'
  else
    echo '[]'
  fi
  exit 0
fi
if [ "$1" = "unlock" ]; then
  rm -f "` + lockPath + `"
  exit 0
fi
if [ "$1" = "check" ]; then
  if [ -f "` + lockPath + `" ]; then
    echo 'check ran before unlock' >&2
    exit 7
  fi
  exit 0
fi
exit 0
`
	if err := os.WriteFile(resticBin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, resticBin)
	svc, err := NewRepoService(repoStore, exec, filepath.Join(tmp, "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	encryptedPassword, err := svc.encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	repoID, err := repoStore.Create(&model.Repository{
		Name:              "local",
		Type:              "local",
		Endpoint:          "/repo",
		PasswordEncrypted: encryptedPassword,
		CheckEnabled:      true,
		CheckCronExpr:     "0 4 * * *",
		CheckArgs:         `["--read-data-subset=10%"]`,
		PruneEnabled:      true,
		PruneCronExpr:     "0 3 * * *",
		PruneArgs:         "[]",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.ScheduledCheckRepo(context.Background(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected check to run after unlock, got exit %d stderr=%q", result.ExitCode, result.Stderr)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(content)
	unlockAt := strings.Index(args, "unlock --remove-all")
	checkAt := strings.LastIndex(args, "check --read-data-subset=10%")
	if unlockAt < 0 {
		t.Fatalf("expected scheduled preflight to unlock foreign locks, got %q", args)
	}
	if checkAt < 0 || unlockAt > checkAt {
		t.Fatalf("expected unlock before check, got %q", args)
	}
}

func TestCurrentInstanceResticLockDetection(t *testing.T) {
	db := setupRepoServiceDB(t)
	repoStore := repository.NewRepoStore(db)
	exec := executor.New(db, "restic")
	svc, err := NewRepoService(repoStore, exec, filepath.Join(t.TempDir(), "key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if !svc.hasCurrentInstanceLiveLock([]resticLockInfo{{Hostname: hostname, PID: os.Getpid()}}) {
		t.Fatal("expected current process lock to be treated as current AutoRestic instance")
	}
	if svc.hasCurrentInstanceLiveLock([]resticLockInfo{{Hostname: "other-host", PID: os.Getpid()}}) {
		t.Fatal("expected other host lock not to be treated as current instance")
	}
}

func TestStoreSnapshotFilesKeepsOnlyDirectChildren(t *testing.T) {
	db := setupRepoServiceDB(t)
	cache := NewRepoCacheService(db, nil)
	defer cache.Stop()

	payload := strings.Join([]string{
		`{"message_type":"snapshot","id":"snap-1"}`,
		`{"name":"data","type":"dir","path":"/data"}`,
		`{"name":"docs","type":"dir","path":"/data/docs"}`,
		`{"name":"deep.txt","type":"file","path":"/data/docs/deep.txt","size":4}`,
		`{"name":"root.txt","type":"file","path":"/data/root.txt","size":8}`,
	}, "\n")

	count, err := cache.storeSnapshotFiles(42, "snap-1", "/data", 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected only direct children to be indexed, got %d", count)
	}

	rows, err := db.Query(`
		SELECT path, parent_path, name
		FROM repository_snapshot_files
		WHERE repo_id=42 AND snapshot_id='snap-1'
		ORDER BY path
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var path, parent, name string
		if err := rows.Scan(&path, &parent, &name); err != nil {
			t.Fatal(err)
		}
		got = append(got, path+"|"+parent+"|"+name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"/data/docs|/data|docs", "/data/root.txt|/data|root.txt"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected indexed rows:\n%s", strings.Join(got, "\n"))
	}
}

func setupRepoServiceDB(t *testing.T) *sql.DB {
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
	return db
}

func waitForTestCondition(t *testing.T, timeout time.Duration, fn func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ok, err := fn()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("condition not satisfied before timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
