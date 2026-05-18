// backend/internal/service/snapshot_service.go
package service

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
)

type SnapshotService struct {
	repoSvc *RepoService
}

func NewSnapshotService(repoSvc *RepoService) *SnapshotService {
	return &SnapshotService{repoSvc: repoSvc}
}

type Snapshot struct {
	ID                  string          `json:"id"`
	ShortID             string          `json:"short_id"`
	Time                string          `json:"time"`
	Hostname            string          `json:"hostname"`
	Username            string          `json:"username,omitempty"`
	UID                 int64           `json:"uid,omitempty"`
	GID                 int64           `json:"gid,omitempty"`
	Tags                []string        `json:"tags"`
	Paths               []string        `json:"paths"`
	Tree                string          `json:"tree"`
	ProgramVersion      string          `json:"program_version,omitempty"`
	Summary             json.RawMessage `json:"summary,omitempty"`
	BackupStart         string          `json:"backup_start,omitempty"`
	BackupEnd           string          `json:"backup_end,omitempty"`
	FilesNew            int64           `json:"files_new,omitempty"`
	FilesChanged        int64           `json:"files_changed,omitempty"`
	FilesUnmodified     int64           `json:"files_unmodified,omitempty"`
	DirsNew             int64           `json:"dirs_new,omitempty"`
	DirsChanged         int64           `json:"dirs_changed,omitempty"`
	DirsUnmodified      int64           `json:"dirs_unmodified,omitempty"`
	DataBlobs           int64           `json:"data_blobs,omitempty"`
	TreeBlobs           int64           `json:"tree_blobs,omitempty"`
	DataAdded           int64           `json:"data_added,omitempty"`
	DataAddedPacked     int64           `json:"data_added_packed,omitempty"`
	TotalFilesProcessed int64           `json:"total_files_processed,omitempty"`
	TotalBytesProcessed int64           `json:"total_bytes_processed,omitempty"`
}

func (s *Snapshot) UnmarshalJSON(data []byte) error {
	type snapshotAlias Snapshot
	var raw struct {
		snapshotAlias
		Summary json.RawMessage `json:"summary"`
	}
	var summary struct {
		BackupStart         string `json:"backup_start"`
		BackupEnd           string `json:"backup_end"`
		FilesNew            int64  `json:"files_new"`
		FilesChanged        int64  `json:"files_changed"`
		FilesUnmodified     int64  `json:"files_unmodified"`
		DirsNew             int64  `json:"dirs_new"`
		DirsChanged         int64  `json:"dirs_changed"`
		DirsUnmodified      int64  `json:"dirs_unmodified"`
		DataBlobs           int64  `json:"data_blobs"`
		TreeBlobs           int64  `json:"tree_blobs"`
		DataAdded           int64  `json:"data_added"`
		DataAddedPacked     int64  `json:"data_added_packed"`
		TotalFilesProcessed int64  `json:"total_files_processed"`
		TotalBytesProcessed int64  `json:"total_bytes_processed"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = Snapshot(raw.snapshotAlias)
	s.Summary = raw.Summary
	if len(raw.Summary) > 0 {
		if err := json.Unmarshal(raw.Summary, &summary); err != nil {
			return err
		}
	}
	s.BackupStart = summary.BackupStart
	s.BackupEnd = summary.BackupEnd
	s.FilesNew = summary.FilesNew
	s.FilesChanged = summary.FilesChanged
	s.FilesUnmodified = summary.FilesUnmodified
	s.DirsNew = summary.DirsNew
	s.DirsChanged = summary.DirsChanged
	s.DirsUnmodified = summary.DirsUnmodified
	s.DataBlobs = summary.DataBlobs
	s.TreeBlobs = summary.TreeBlobs
	s.DataAdded = summary.DataAdded
	s.DataAddedPacked = summary.DataAddedPacked
	s.TotalFilesProcessed = summary.TotalFilesProcessed
	s.TotalBytesProcessed = summary.TotalBytesProcessed
	return nil
}

type FileEntry struct {
	Name    string          `json:"name"`
	Path    string          `json:"path"`
	Type    string          `json:"type"` // dir, file
	Size    int64           `json:"size"`
	Mode    json.RawMessage `json:"mode,omitempty"`
	ModTime string          `json:"mtime"`
}

func (s *SnapshotService) ListSnapshots(ctx context.Context, repoID int64) ([]Snapshot, error) {
	return s.ListSnapshotsCached(ctx, repoID, false)
}

func (s *SnapshotService) ListSnapshotIndex(repoID int64, page, pageSize int, refresh bool) (SnapshotIndexPage, error) {
	return s.repoSvc.Cache().ListSnapshotIndex(repoID, page, pageSize, refresh)
}

func (s *SnapshotService) ListSnapshotIndexFiltered(repoID int64, page, pageSize int, refresh bool, updateFilter string) (SnapshotIndexPage, error) {
	return s.repoSvc.Cache().ListSnapshotIndexFiltered(repoID, page, pageSize, refresh, updateFilter)
}

func (s *SnapshotService) ListSnapshotsCached(ctx context.Context, repoID int64, refresh bool) ([]Snapshot, error) {
	page, err := s.repoSvc.Cache().ListSnapshotIndex(repoID, 1, 200, refresh)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *SnapshotService) ListSnapshotsRaw(ctx context.Context, repoID int64) ([]Snapshot, error) {
	result, err := s.repoSvc.RunResticCommand(ctx, repoID, "system_query", []string{"snapshots", "--json"}, nil)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, nil
	}

	return parseSnapshots(result.Stdout)
}

func (s *SnapshotService) ListFiles(ctx context.Context, repoID int64, snapshotID string) ([]FileEntry, error) {
	return s.ListFilesCached(ctx, repoID, snapshotID, "", false)
}

func (s *SnapshotService) ListFilesCached(ctx context.Context, repoID int64, snapshotID, path string, refresh bool) ([]FileEntry, error) {
	page, err := s.repoSvc.Cache().ListSnapshotFiles(repoID, snapshotID, path, refresh)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *SnapshotService) ListFilesView(repoID int64, snapshotID, path string, refresh bool) (SnapshotFilesPage, error) {
	return s.repoSvc.Cache().ListSnapshotFiles(repoID, snapshotID, path, refresh)
}

func (s *SnapshotService) ListFilesRaw(ctx context.Context, repoID int64, snapshotID string) ([]FileEntry, error) {
	result, err := s.repoSvc.RunResticCommand(ctx, repoID, "system_query", []string{"ls", "--json", snapshotID}, nil)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, nil
	}

	return parseResticFileEntries(result.Stdout)
}

func (s *SnapshotService) Diff(ctx context.Context, repoID int64, snap1, snap2 string) (string, error) {
	result, err := s.repoSvc.RunResticCommand(ctx, repoID, "system_query", []string{"diff", snap1, snap2}, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout + result.Stderr, nil
}

func (s *SnapshotService) Restore(ctx context.Context, repoID int64, snapshotID, targetPath string, includes []string) (executor.ExecResult, error) {
	args := []string{"restore", snapshotID, "--target", targetPath}
	for _, inc := range includes {
		args = append(args, "--include", inc)
	}
	return s.repoSvc.RunResticCommand(ctx, repoID, "manual", args, nil)
}

func (s *SnapshotService) RestoreAsync(repoID int64, snapshotID, targetPath string, includes []string) (int64, error) {
	args := []string{"restore", snapshotID, "--target", targetPath}
	for _, inc := range includes {
		args = append(args, "--include", inc)
	}
	return s.runSnapshotCommandAsync(repoID, args, nil)
}

func (s *SnapshotService) Find(ctx context.Context, repoID int64, pattern string) (string, error) {
	result, err := s.repoSvc.RunResticCommand(ctx, repoID, "system_query", []string{"find", pattern}, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout + result.Stderr, nil
}

func (s *SnapshotService) Forget(ctx context.Context, repoID int64, snapshotID string) (executor.ExecResult, error) {
	result, err := s.repoSvc.RunResticCommand(ctx, repoID, "manual", []string{"forget", snapshotID}, nil)
	if err == nil && result.ExitCode == 0 {
		s.repoSvc.Cache().MarkStale(repoID, syncDomainCore, syncDomainStats, syncDomainFiles)
		_, _ = s.repoSvc.Cache().QueueCoreSync(repoID)
	}
	return result, err
}

func (s *SnapshotService) ForgetAsync(repoID int64, snapshotID string) (int64, error) {
	return s.runSnapshotCommandAsync(repoID, []string{"forget", snapshotID}, func(result executor.ExecResult, err error) {
		if err == nil && result.ExitCode == 0 {
			s.repoSvc.Cache().MarkStale(repoID, syncDomainCore, syncDomainStats, syncDomainFiles)
			_, _ = s.repoSvc.Cache().QueueCoreSync(repoID)
		}
	})
}

func parseResticFileEntries(raw string) ([]FileEntry, error) {
	var files []FileEntry
	if strings.TrimSpace(raw) == "" {
		return files, nil
	}
	if err := json.Unmarshal([]byte(raw), &files); err == nil {
		return files, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry FileEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		files = append(files, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func parseSnapshots(raw string) ([]Snapshot, error) {
	var snaps []Snapshot
	if strings.TrimSpace(raw) == "" {
		return snaps, nil
	}
	if err := json.Unmarshal([]byte(raw), &snaps); err != nil {
		return nil, err
	}
	return snaps, nil
}

func (s *SnapshotService) runSnapshotCommandAsync(repoID int64, args []string, onDone func(executor.ExecResult, error)) (int64, error) {
	started := make(chan int64, 1)
	startErr := make(chan error, 1)
	go func() {
		result, err := s.repoSvc.runResticCommand(context.Background(), repoID, nil, "manual", args, nil, started)
		if onDone != nil {
			onDone(result, err)
		}
		if err != nil {
			startErr <- err
			return
		}
	}()

	select {
	case logID := <-started:
		return logID, nil
	case err := <-startErr:
		return 0, err
	case <-time.After(5 * time.Second):
		return 0, context.DeadlineExceeded
	}
}
