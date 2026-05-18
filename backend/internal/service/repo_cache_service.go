package service

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
)

const (
	syncDomainCore  = "core"
	syncDomainFiles = "files"
	syncDomainStats = "stats"
	syncDomainKeys  = "keys"
	syncDomainCheck = "check"

	syncStatusRunning     = "running"
	syncStatusSuccess     = "success"
	syncStatusFailed      = "failed"
	syncStatusStale       = "stale"
	syncStatusInterrupted = "interrupted"
	syncStatusPartial     = "partial"

	indexBatchSize = 1000

	plannerInterval             = time.Hour
	maxPlannerQueuesPerRound    = 4
	filesPhaseRootsPrewarmed    = "roots_prewarmed"
	filesPhasePendingBackground = "background_pending"
)

type SyncState struct {
	RepoID        int64      `json:"repo_id"`
	Domain        string     `json:"domain"`
	Status        string     `json:"status"`
	Phase         string     `json:"phase"`
	Progress      int        `json:"progress"`
	Generation    int64      `json:"generation"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	LogID         *int64     `json:"log_id,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type SyncStateView struct {
	Status       string     `json:"status"`
	Phase        string     `json:"phase,omitempty"`
	Progress     int        `json:"progress,omitempty"`
	Generation   int64      `json:"generation,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	Error        string     `json:"error,omitempty"`
	LogID        *int64     `json:"log_id,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type SnapshotIndexPage struct {
	Items                []Snapshot `json:"items"`
	Total                int        `json:"total"`
	Page                 int        `json:"page"`
	PageSize             int        `json:"page_size"`
	Indexing             bool       `json:"indexing"`
	LastIndexed          *time.Time `json:"last_indexed_at,omitempty"`
	Error                string     `json:"error,omitempty"`
	Stale                bool       `json:"stale"`
	Status               string     `json:"status,omitempty"`
	SyncStatus           string     `json:"sync_status,omitempty"`
	Partial              bool       `json:"partial"`
	IndexedSnapshotCount int        `json:"indexed_snapshot_count"`
}

type SnapshotFilesPage struct {
	Items     []FileEntry `json:"items"`
	Indexing  bool        `json:"indexing"`
	Stale     bool        `json:"stale"`
	Error     string      `json:"error,omitempty"`
	IndexedAt *time.Time  `json:"indexed_at,omitempty"`
	Total     int         `json:"total"`
}

type RepoStatsView struct {
	Data      json.RawMessage `json:"data,omitempty"`
	Indexing  bool            `json:"indexing"`
	Stale     bool            `json:"stale"`
	Error     string          `json:"error,omitempty"`
	IndexedAt *time.Time      `json:"indexed_at,omitempty"`
}

type RepoKeysView struct {
	Items     []json.RawMessage `json:"items"`
	Indexing  bool              `json:"indexing"`
	Stale     bool              `json:"stale"`
	Error     string            `json:"error,omitempty"`
	IndexedAt *time.Time        `json:"indexed_at,omitempty"`
	Total     int               `json:"total"`
}

type syncLoad struct {
	done    chan struct{}
	started chan int64
	err     error
	logID   int64
}

type fileIndexRequest struct {
	snapshotID string
	path       string
	force      bool
	prewarm    bool
}

type refreshCandidate struct {
	repoID    int64
	domain    string
	priority  int
	dueAt     time.Time
	updatedAt time.Time
}

type RepoCacheService struct {
	db      *sql.DB
	repoSvc *RepoService

	mu       sync.Mutex
	active   map[string]*syncLoad
	fileWork map[int64]map[string]fileIndexRequest

	plannerOnce sync.Once
	stopPlanner chan struct{}
}

func NewRepoCacheService(db *sql.DB, repoSvc *RepoService) *RepoCacheService {
	svc := &RepoCacheService{
		db:          db,
		repoSvc:     repoSvc,
		active:      map[string]*syncLoad{},
		fileWork:    map[int64]map[string]fileIndexRequest{},
		stopPlanner: make(chan struct{}),
	}
	return svc
}

func (s *RepoCacheService) StartPlanner() {
	s.plannerOnce.Do(func() {
		go s.plannerLoop()
	})
}

func (s *RepoCacheService) Stop() {
	select {
	case <-s.stopPlanner:
	default:
		close(s.stopPlanner)
	}
}

func (s *RepoCacheService) plannerLoop() {
	// The planner only resumes explicit stale/failed/interrupted metadata states.
	// Running states are preserved so long-running repositories remain visible as running.
	_ = s.RefreshAll(context.Background())

	ticker := time.NewTicker(plannerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopPlanner:
			return
		case <-ticker.C:
			_ = s.RefreshAll(context.Background())
		}
	}
}

func (s *RepoCacheService) RefreshAll(ctx context.Context) error {
	if s.repoSvc == nil || s.repoSvc.store == nil {
		return nil
	}
	repos, err := s.repoSvc.store.List()
	if err != nil {
		return err
	}
	candidates, err := s.planRefreshRound(ctx, repos, time.Now())
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		switch candidate.domain {
		case syncDomainCore:
			_, _ = s.QueueCoreSync(candidate.repoID)
		case syncDomainStats:
			_, _ = s.QueueStatsSync(candidate.repoID)
		case syncDomainKeys:
			_, _ = s.QueueKeysSync(candidate.repoID)
		case syncDomainFiles:
			_, _ = s.QueueFilesSync(candidate.repoID)
		}
	}
	return nil
}

func (s *RepoCacheService) planRefreshRound(ctx context.Context, repos []model.Repository, now time.Time) ([]refreshCandidate, error) {
	candidates := make([]refreshCandidate, 0, len(repos))
	for _, repo := range repos {
		candidate, ok, err := s.nextRefreshCandidate(ctx, repo.ID, now)
		if err != nil {
			return nil, err
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if !candidates[i].dueAt.Equal(candidates[j].dueAt) {
			return candidates[i].dueAt.Before(candidates[j].dueAt)
		}
		if !candidates[i].updatedAt.Equal(candidates[j].updatedAt) {
			return candidates[i].updatedAt.Before(candidates[j].updatedAt)
		}
		if candidates[i].repoID != candidates[j].repoID {
			return candidates[i].repoID < candidates[j].repoID
		}
		return candidates[i].domain < candidates[j].domain
	})

	if len(candidates) > maxPlannerQueuesPerRound {
		candidates = candidates[:maxPlannerQueuesPerRound]
	}
	return candidates, nil
}

func (s *RepoCacheService) nextRefreshCandidate(ctx context.Context, repoID int64, now time.Time) (refreshCandidate, bool, error) {
	domains := []string{syncDomainCore}
	for _, domain := range domains {
		candidate, ok, err := s.refreshCandidateForDomain(ctx, repoID, domain, now)
		if err != nil {
			return refreshCandidate{}, false, err
		}
		if ok {
			return candidate, true, nil
		}
	}
	return refreshCandidate{}, false, nil
}

func (s *RepoCacheService) refreshCandidateForDomain(ctx context.Context, repoID int64, domain string, now time.Time) (refreshCandidate, bool, error) {
	if s.isActive(repoID, domain) {
		return refreshCandidate{}, false, nil
	}
	if domain == syncDomainFiles {
		if !s.shouldQueueFiles(repoID, ctx) {
			return refreshCandidate{}, false, nil
		}
		state, ok, err := s.getSyncState(repoID, domain)
		if err != nil && !isNoSuchTableError(err) {
			return refreshCandidate{}, false, err
		}
		updatedAt := now
		if ok {
			updatedAt = state.UpdatedAt
		}
		return refreshCandidate{repoID: repoID, domain: domain, priority: 0, dueAt: updatedAt, updatedAt: updatedAt}, true, nil
	}

	state, ok, err := s.getSyncState(repoID, domain)
	if err != nil && !isNoSuchTableError(err) {
		return refreshCandidate{}, false, err
	}
	if !ok {
		return refreshCandidate{}, false, nil
	}
	if state.Status == syncStatusStale || state.Status == syncStatusFailed || state.Status == syncStatusInterrupted {
		return refreshCandidate{repoID: repoID, domain: domain, priority: 0, dueAt: state.UpdatedAt, updatedAt: state.UpdatedAt}, true, nil
	}
	return refreshCandidate{}, false, nil
}

func (s *RepoCacheService) shouldQueueFiles(repoID int64, ctx context.Context) bool {
	state, ok, err := s.getSyncState(repoID, syncDomainFiles)
	if err != nil && !isNoSuchTableError(err) {
		return false
	}
	if ok && (state.Status == syncStatusStale || state.Status == syncStatusFailed || state.Status == syncStatusInterrupted) {
		if s.hasStaleFileIndexes(ctx, repoID) {
			return true
		}
		paths, err := s.collectRootPathsForCurrentSnapshots(repoID)
		return err == nil && len(paths) > 0
	}
	return s.hasStaleFileIndexes(ctx, repoID)
}

func (s *RepoCacheService) hasStaleFileIndexes(ctx context.Context, repoID int64) bool {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM repository_snapshot_file_indexes WHERE repo_id=? AND stale=1`,
		repoID,
	).Scan(&count)
	return err == nil && count > 0
}

func (s *RepoCacheService) QueueSync(repoID int64, domain string) (int64, error) {
	switch domain {
	case syncDomainCore:
		return s.QueueCoreSync(repoID)
	case syncDomainStats:
		return s.QueueStatsSync(repoID)
	case syncDomainKeys:
		return s.QueueKeysSync(repoID)
	case syncDomainFiles:
		return s.QueueFilesSync(repoID)
	case "all":
		return s.QueueInitialImport(repoID)
	default:
		return 0, fmt.Errorf("unsupported sync domain %q", domain)
	}
}

func (s *RepoCacheService) QueueInitialImport(repoID int64) (int64, error) {
	_ = s.markSyncStateStale(repoID, syncDomainStats, "stats refresh is on demand for large repositories")
	_ = s.markSyncStateStale(repoID, syncDomainKeys, "key list refresh is on demand for large repositories")
	return s.QueueCoreSync(repoID)
}

func (s *RepoCacheService) QueueCoreSync(repoID int64) (int64, error) {
	return s.queueDomainSync(repoID, syncDomainCore, s.runCoreSync)
}

func (s *RepoCacheService) QueueStatsSync(repoID int64) (int64, error) {
	return s.queueDomainSync(repoID, syncDomainStats, s.runStatsSync)
}

func (s *RepoCacheService) QueueKeysSync(repoID int64) (int64, error) {
	return s.queueDomainSync(repoID, syncDomainKeys, s.runKeysSync)
}

func (s *RepoCacheService) QueueFilesSync(repoID int64) (int64, error) {
	paths, err := s.collectFileSyncTargets(repoID)
	if err != nil {
		return 0, err
	}
	if len(paths) == 0 {
		paths, err = s.collectRootPathsForCurrentSnapshots(repoID)
		if err != nil {
			return 0, err
		}
	}
	if len(paths) == 0 {
		return 0, nil
	}
	for _, req := range paths {
		s.enqueueFileIndex(repoID, req)
	}
	return s.ensureFileWorker(repoID)
}

func (s *RepoCacheService) QueueLatestSnapshotRootPrewarm(repoID int64) (int64, error) {
	paths, err := s.collectRootPathsForCurrentSnapshots(repoID)
	if err != nil {
		return 0, err
	}
	if len(paths) == 0 {
		return 0, nil
	}
	for _, req := range paths {
		req.prewarm = true
		s.enqueueFileIndex(repoID, req)
	}
	return s.ensureFileWorker(repoID)
}

func (s *RepoCacheService) EnsureSnapshotPathIndex(repoID int64, snapshotID, path string, refresh bool) {
	path = normalizeSnapshotPath(path)
	state, ok, err := s.getFileIndexState(repoID, snapshotID, path)
	if err != nil {
		return
	}
	if refresh || !ok || state.Status == syncStatusStale || state.Status == syncStatusFailed || state.Status == syncStatusInterrupted {
		s.enqueueFileIndex(repoID, fileIndexRequest{snapshotID: snapshotID, path: path, force: refresh})
		_, _ = s.ensureFileWorker(repoID)
	}
}

func (s *RepoCacheService) enqueueFileIndex(repoID int64, req fileIndexRequest) {
	key := req.snapshotID + "|" + req.path
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.fileWork[repoID]
	if queue == nil {
		queue = map[string]fileIndexRequest{}
		s.fileWork[repoID] = queue
	}
	if existing, ok := queue[key]; ok {
		if req.force && !existing.force {
			existing.force = true
		}
		if !req.prewarm && existing.prewarm {
			existing.prewarm = false
		}
		queue[key] = existing
		return
	}
	queue[key] = req
}

func (s *RepoCacheService) ensureFileWorker(repoID int64) (int64, error) {
	return s.queueDomainSync(repoID, syncDomainFiles, s.runFilesSync)
}

func (s *RepoCacheService) queueDomainSync(repoID int64, domain string, runner func(context.Context, int64, *syncLoad) error) (int64, error) {
	load, owner := s.beginLoad(repoID, domain)
	if owner {
		go func() {
			err := runner(context.Background(), repoID, load)
			s.finishLoad(repoID, domain, load, err)
		}()
	}

	if logID, _ := s.loadResult(load); logID > 0 {
		return logID, nil
	}

	select {
	case logID := <-load.started:
		return logID, nil
	case <-load.done:
		logID, loadErr := s.loadResult(load)
		if logID > 0 {
			return logID, loadErr
		}
		return 0, loadErr
	case <-time.After(5 * time.Second):
		state, _, err := s.getSyncState(repoID, domain)
		if err == nil && state.LogID != nil {
			return *state.LogID, nil
		}
		return 0, nil
	}
}

func (s *RepoCacheService) beginLoad(repoID int64, domain string) (*syncLoad, bool) {
	key := syncKey(repoID, domain)
	s.mu.Lock()
	defer s.mu.Unlock()
	if load, ok := s.active[key]; ok {
		return load, false
	}
	load := &syncLoad{done: make(chan struct{}), started: make(chan int64, 1)}
	s.active[key] = load
	return load, true
}

func (s *RepoCacheService) finishLoad(repoID int64, domain string, load *syncLoad, err error) {
	key := syncKey(repoID, domain)
	s.mu.Lock()
	load.err = err
	delete(s.active, key)
	close(load.done)
	hasPendingFileWork := domain == syncDomainFiles && len(s.fileWork[repoID]) > 0
	s.mu.Unlock()
	if hasPendingFileWork {
		go func() {
			_, _ = s.ensureFileWorker(repoID)
		}()
	}
}

func (s *RepoCacheService) isActive(repoID int64, domain string) bool {
	key := syncKey(repoID, domain)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.active[key]
	return ok
}

func (s *RepoCacheService) loadResult(load *syncLoad) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return load.logID, load.err
}

func (s *RepoCacheService) announceStarted(load *syncLoad, logID int64) {
	if logID <= 0 {
		return
	}
	s.mu.Lock()
	if load.logID == 0 {
		load.logID = logID
		select {
		case load.started <- logID:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *RepoCacheService) runCoreSync(ctx context.Context, repoID int64, load *syncLoad) error {
	generation, err := s.nextGeneration(repoID, syncDomainCore)
	if err != nil {
		return err
	}
	if err := s.setSyncState(repoID, syncDomainCore, syncStatusRunning, "config", 10, 0, nil, "", nil); err != nil {
		return err
	}

	configPayload, configLogID, err := s.runSyncCommand(ctx, repoID, "config", []string{"cat", "config"}, load)
	if configLogID > 0 {
		_ = s.setSyncState(repoID, syncDomainCore, syncStatusRunning, "config", 25, 0, nil, "", &configLogID)
	}
	if err != nil {
		_ = s.setSyncState(repoID, syncDomainCore, syncStatusFailed, "config", 25, 0, nil, err.Error(), &configLogID)
		return err
	}
	if err := s.storeResticConfig(repoID, generation, configPayload); err != nil {
		_ = s.setSyncState(repoID, syncDomainCore, syncStatusFailed, "config", 25, 0, nil, err.Error(), &configLogID)
		return err
	}

	if err := s.setSyncState(repoID, syncDomainCore, syncStatusRunning, "snapshots", 50, 0, nil, "", &configLogID); err != nil {
		return err
	}
	snapshotPayload, snapshotLogID, err := s.runSyncCommand(ctx, repoID, "snapshots", []string{"snapshots", "--json"}, load)
	if snapshotLogID > 0 {
		_ = s.setSyncState(repoID, syncDomainCore, syncStatusRunning, "snapshots", 75, 0, nil, "", &snapshotLogID)
	}
	if err != nil {
		_ = s.setSyncState(repoID, syncDomainCore, syncStatusFailed, "snapshots", 75, 0, nil, err.Error(), &snapshotLogID)
		return err
	}
	if err := s.replaceSnapshots(repoID, generation, snapshotPayload); err != nil {
		_ = s.setSyncState(repoID, syncDomainCore, syncStatusFailed, "snapshots", 75, 0, nil, err.Error(), &snapshotLogID)
		return err
	}
	if err := s.finalizeCoreGeneration(repoID, generation); err != nil {
		_ = s.setSyncState(repoID, syncDomainCore, syncStatusFailed, "snapshots", 90, 0, nil, err.Error(), &snapshotLogID)
		return err
	}
	if err := s.setSyncState(repoID, syncDomainCore, syncStatusSuccess, "", 100, generation, timePtr(time.Now()), "", &snapshotLogID); err != nil {
		return err
	}
	_ = s.markFilesDomainStale(repoID, "core sync completed")
	_, _ = s.QueueLatestSnapshotRootPrewarm(repoID)
	return nil
}

func (s *RepoCacheService) runStatsSync(ctx context.Context, repoID int64, load *syncLoad) error {
	generation, err := s.nextGeneration(repoID, syncDomainStats)
	if err != nil {
		return err
	}
	if err := s.setSyncState(repoID, syncDomainStats, syncStatusRunning, "stats", 10, 0, nil, "", nil); err != nil {
		return err
	}
	payload, logID, err := s.runSyncCommand(ctx, repoID, "stats", []string{"stats", "--json"}, load)
	if logID > 0 {
		_ = s.setSyncState(repoID, syncDomainStats, syncStatusRunning, "stats", 60, 0, nil, "", &logID)
	}
	if err != nil {
		_ = s.setSyncState(repoID, syncDomainStats, syncStatusFailed, "stats", 60, 0, nil, err.Error(), &logID)
		return err
	}
	if err := s.storeStats(repoID, generation, payload); err != nil {
		_ = s.setSyncState(repoID, syncDomainStats, syncStatusFailed, "stats", 60, 0, nil, err.Error(), &logID)
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM repository_stats WHERE repo_id=? AND generation<>?`, repoID, generation); err != nil {
		_ = s.setSyncState(repoID, syncDomainStats, syncStatusFailed, "stats", 80, 0, nil, err.Error(), &logID)
		return err
	}
	return s.setSyncState(repoID, syncDomainStats, syncStatusSuccess, "", 100, generation, timePtr(time.Now()), "", &logID)
}

func (s *RepoCacheService) runKeysSync(ctx context.Context, repoID int64, load *syncLoad) error {
	generation, err := s.nextGeneration(repoID, syncDomainKeys)
	if err != nil {
		return err
	}
	if err := s.setSyncState(repoID, syncDomainKeys, syncStatusRunning, "keys", 10, 0, nil, "", nil); err != nil {
		return err
	}
	payload, logID, err := s.runSyncCommand(ctx, repoID, "keys", []string{"key", "list", "--json"}, load)
	if logID > 0 {
		_ = s.setSyncState(repoID, syncDomainKeys, syncStatusRunning, "keys", 60, 0, nil, "", &logID)
	}
	if err != nil {
		_ = s.setSyncState(repoID, syncDomainKeys, syncStatusFailed, "keys", 60, 0, nil, err.Error(), &logID)
		return err
	}
	if err := s.storeKeys(repoID, generation, payload); err != nil {
		_ = s.setSyncState(repoID, syncDomainKeys, syncStatusFailed, "keys", 60, 0, nil, err.Error(), &logID)
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM repository_keys WHERE repo_id=? AND generation<>?`, repoID, generation); err != nil {
		_ = s.setSyncState(repoID, syncDomainKeys, syncStatusFailed, "keys", 80, 0, nil, err.Error(), &logID)
		return err
	}
	return s.setSyncState(repoID, syncDomainKeys, syncStatusSuccess, "", 100, generation, timePtr(time.Now()), "", &logID)
}

func (s *RepoCacheService) runFilesSync(ctx context.Context, repoID int64, load *syncLoad) error {
	generation, err := s.nextGeneration(repoID, syncDomainFiles)
	if err != nil {
		return err
	}
	var lastLogID int64
	if err := s.setSyncState(repoID, syncDomainFiles, syncStatusRunning, "queue", 5, 0, nil, "", nil); err != nil {
		return err
	}

	processed := false
	onlyPrewarm := true
	for {
		req, ok := s.dequeueFileIndex(repoID)
		if !ok {
			break
		}
		processed = true
		if !req.prewarm {
			onlyPrewarm = false
		}
		progressPhase := req.snapshotID + ":" + req.path
		if err := s.setFileIndexState(repoID, req.snapshotID, req.path, syncStatusRunning, 0, nil, "", true, 0); err != nil {
			return err
		}
		if err := s.setSyncState(repoID, syncDomainFiles, syncStatusRunning, progressPhase, 20, 0, nil, "", int64Ptr(lastLogID)); err != nil {
			return err
		}

		if req.path == "" {
			count, storeErr := s.storeSnapshotFileRoots(repoID, req.snapshotID, generation)
			if storeErr != nil {
				_ = s.setFileIndexState(repoID, req.snapshotID, req.path, syncStatusFailed, 0, nil, storeErr.Error(), true, 0)
				_ = s.setSyncState(repoID, syncDomainFiles, syncStatusFailed, progressPhase, 80, 0, nil, storeErr.Error(), int64Ptr(lastLogID))
				continue
			}
			now := time.Now()
			if err := s.setFileIndexState(repoID, req.snapshotID, req.path, syncStatusSuccess, count, &now, "", false, generation); err != nil {
				return err
			}
			continue
		}

		args := []string{"ls", "--json", req.snapshotID}
		if req.path != "" {
			args = append(args, req.path)
		}
		payload, logID, runErr := s.runSyncCommand(ctx, repoID, "files", args, load)
		if logID > 0 {
			lastLogID = logID
			_ = s.setSyncState(repoID, syncDomainFiles, syncStatusRunning, progressPhase, 60, 0, nil, "", &lastLogID)
		}
		if runErr != nil {
			_ = s.setFileIndexState(repoID, req.snapshotID, req.path, syncStatusFailed, 0, nil, runErr.Error(), true, 0)
			_ = s.setSyncState(repoID, syncDomainFiles, syncStatusFailed, progressPhase, 60, 0, nil, runErr.Error(), int64Ptr(lastLogID))
			continue
		}
		count, storeErr := s.storeSnapshotFiles(repoID, req.snapshotID, req.path, generation, payload)
		if storeErr != nil {
			_ = s.setFileIndexState(repoID, req.snapshotID, req.path, syncStatusFailed, 0, nil, storeErr.Error(), true, 0)
			_ = s.setSyncState(repoID, syncDomainFiles, syncStatusFailed, progressPhase, 80, 0, nil, storeErr.Error(), int64Ptr(lastLogID))
			continue
		}
		now := time.Now()
		if err := s.setFileIndexState(repoID, req.snapshotID, req.path, syncStatusSuccess, count, &now, "", false, generation); err != nil {
			return err
		}
	}

	status := syncStatusSuccess
	phase := ""
	successAt := timePtr(time.Now())
	if s.hasStaleFileIndexes(ctx, repoID) {
		status = syncStatusStale
		phase = filesPhasePendingBackground
	}
	if processed && onlyPrewarm && status == syncStatusSuccess {
		status = syncStatusStale
		phase = filesPhaseRootsPrewarmed
	}
	return s.setSyncState(repoID, syncDomainFiles, status, phase, 100, generation, successAt, "", int64Ptr(lastLogID))
}

func (s *RepoCacheService) dequeueFileIndex(repoID int64) (fileIndexRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.fileWork[repoID]
	for key, req := range queue {
		delete(queue, key)
		if len(queue) == 0 {
			delete(s.fileWork, repoID)
		}
		return req, true
	}
	return fileIndexRequest{}, false
}

func (s *RepoCacheService) collectFileSyncTargets(repoID int64) ([]fileIndexRequest, error) {
	rows, err := s.db.Query(
		`SELECT snapshot_id, path
		 FROM repository_snapshot_file_indexes
		 WHERE repo_id=? AND stale=1
		 ORDER BY updated_at ASC`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []fileIndexRequest
	for rows.Next() {
		var req fileIndexRequest
		if err := rows.Scan(&req.snapshotID, &req.path); err != nil {
			return nil, err
		}
		items = append(items, req)
	}
	return items, rows.Err()
}

func (s *RepoCacheService) collectRootPathsForCurrentSnapshots(repoID int64) ([]fileIndexRequest, error) {
	generation, err := s.currentGeneration(repoID, syncDomainCore)
	if err != nil {
		return nil, err
	}
	if generation == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT snapshot_id FROM repository_snapshots
		 WHERE repo_id=? AND generation=?
		 ORDER BY time DESC, snapshot_id DESC
		 LIMIT 3`,
		repoID, generation,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []fileIndexRequest
	for rows.Next() {
		var snapshotID string
		if err := rows.Scan(&snapshotID); err != nil {
			return nil, err
		}
		items = append(items, fileIndexRequest{snapshotID: snapshotID, path: "", prewarm: true})
	}
	return items, rows.Err()
}

func (s *RepoCacheService) runSyncCommand(ctx context.Context, repoID int64, phase string, args []string, load *syncLoad) (string, int64, error) {
	started := make(chan int64, 1)
	result, err := s.repoSvc.runResticCommand(ctx, repoID, nil, "system_query", args, nil, started)
	logID := result.LogID
	select {
	case startedLogID := <-started:
		logID = startedLogID
	default:
	}
	s.announceStarted(load, logID)
	payload, payloadErr := commandPayload(result, err)
	if payloadErr != nil {
		return result.Stdout, logID, payloadErr
	}
	return payload, logID, nil
}

func (s *RepoCacheService) ListSnapshotIndex(repoID int64, page, pageSize int, refresh bool) (SnapshotIndexPage, error) {
	return s.ListSnapshotIndexFiltered(repoID, page, pageSize, refresh, "")
}

func (s *RepoCacheService) ListSnapshotIndexFiltered(repoID int64, page, pageSize int, refresh bool, updateFilter string) (SnapshotIndexPage, error) {
	if refresh {
		_, _ = s.QueueCoreSync(repoID)
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	state, ok, err := s.getSyncState(repoID, syncDomainCore)
	if err != nil && !isNoSuchTableError(err) {
		return SnapshotIndexPage{}, err
	}
	if !ok || state.Status == "" {
		state.Status = syncStatusStale
	}

	filterSQL := snapshotUpdateFilterSQL(updateFilter)
	whereSQL := "repo_id=? AND generation=?"
	if filterSQL != "" {
		whereSQL += " AND " + filterSQL
	}

	var total int
	if state.Generation > 0 {
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM repository_snapshots WHERE `+whereSQL,
			repoID, state.Generation,
		).Scan(&total); err != nil && !isNoSuchTableError(err) {
			return SnapshotIndexPage{}, err
		}
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.Query(
		`SELECT snapshot_id, short_id, time, hostname, username, uid, gid, tags, paths, tree,
		        program_version, summary, backup_start, backup_end, files_new, files_changed,
		        files_unmodified, dirs_new, dirs_changed, dirs_unmodified, data_blobs, tree_blobs,
		        data_added, data_added_packed, total_files_processed, total_bytes_processed
		 FROM repository_snapshots
		 WHERE `+whereSQL+`
		 ORDER BY time DESC, snapshot_id DESC
		 LIMIT ? OFFSET ?`,
		repoID, state.Generation, pageSize, offset,
	)
	if err != nil && !isNoSuchTableError(err) {
		return SnapshotIndexPage{}, err
	}
	items := []Snapshot{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var snap Snapshot
			var tagsRaw, pathsRaw, summaryRaw string
			if err := rows.Scan(
				&snap.ID, &snap.ShortID, &snap.Time, &snap.Hostname, &snap.Username, &snap.UID, &snap.GID,
				&tagsRaw, &pathsRaw, &snap.Tree, &snap.ProgramVersion, &summaryRaw,
				&snap.BackupStart, &snap.BackupEnd, &snap.FilesNew, &snap.FilesChanged, &snap.FilesUnmodified,
				&snap.DirsNew, &snap.DirsChanged, &snap.DirsUnmodified, &snap.DataBlobs, &snap.TreeBlobs,
				&snap.DataAdded, &snap.DataAddedPacked, &snap.TotalFilesProcessed, &snap.TotalBytesProcessed,
			); err != nil {
				return SnapshotIndexPage{}, err
			}
			_ = json.Unmarshal([]byte(tagsRaw), &snap.Tags)
			_ = json.Unmarshal([]byte(pathsRaw), &snap.Paths)
			if strings.TrimSpace(summaryRaw) != "" && summaryRaw != "{}" {
				snap.Summary = json.RawMessage(summaryRaw)
			}
			items = append(items, snap)
		}
		if err := rows.Err(); err != nil {
			return SnapshotIndexPage{}, err
		}
	}

	active := s.isActive(repoID, syncDomainCore)
	stale := active || state.Generation == 0 || state.Status == syncStatusFailed || state.Status == syncStatusStale || state.Status == syncStatusInterrupted
	return SnapshotIndexPage{
		Items:                items,
		Total:                total,
		Page:                 page,
		PageSize:             pageSize,
		Indexing:             active || state.Status == syncStatusRunning,
		LastIndexed:          state.LastSuccessAt,
		Error:                state.LastError,
		Stale:                stale,
		Status:               state.Status,
		SyncStatus:           state.Status,
		Partial:              stale && total > 0,
		IndexedSnapshotCount: total,
	}, nil
}

func snapshotUpdateFilterSQL(filter string) string {
	updated := `(files_new<>0 OR files_changed<>0 OR dirs_new<>0 OR dirs_changed<>0 OR data_added<>0 OR data_added_packed<>0)`
	known := `(summary<>'{}' OR backup_start<>'' OR backup_end<>'' OR files_new<>0 OR files_changed<>0 OR files_unmodified<>0 OR dirs_new<>0 OR dirs_changed<>0 OR dirs_unmodified<>0 OR data_added<>0 OR data_added_packed<>0 OR total_files_processed<>0 OR total_bytes_processed<>0)`
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "updated":
		return updated
	case "unchanged":
		return known + " AND NOT " + updated
	case "unknown":
		return "NOT " + known
	default:
		return ""
	}
}

func (s *RepoCacheService) StoreBackupSummary(repoID int64, payload string) error {
	summary, ok, err := parseBackupSummaryPayload(payload)
	if err != nil || !ok {
		return err
	}
	state, ok, err := s.getSyncState(repoID, syncDomainCore)
	if err != nil && !isNoSuchTableError(err) {
		return err
	}
	generation := int64(0)
	if ok {
		generation = state.Generation
	}
	shortID := summary.SnapshotID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	_, err = s.db.Exec(
		`INSERT INTO repository_snapshots
		 (repo_id, snapshot_id, short_id, time, summary, backup_start, backup_end, files_new, files_changed,
		  files_unmodified, dirs_new, dirs_changed, dirs_unmodified, data_blobs, tree_blobs, data_added,
		  data_added_packed, total_files_processed, total_bytes_processed, raw_json, indexed_at, generation)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, snapshot_id, generation) DO UPDATE SET
		   summary=excluded.summary,
		   backup_start=excluded.backup_start,
		   backup_end=excluded.backup_end,
		   files_new=excluded.files_new,
		   files_changed=excluded.files_changed,
		   files_unmodified=excluded.files_unmodified,
		   dirs_new=excluded.dirs_new,
		   dirs_changed=excluded.dirs_changed,
		   dirs_unmodified=excluded.dirs_unmodified,
		   data_blobs=excluded.data_blobs,
		   tree_blobs=excluded.tree_blobs,
		   data_added=excluded.data_added,
		   data_added_packed=excluded.data_added_packed,
		   total_files_processed=excluded.total_files_processed,
		   total_bytes_processed=excluded.total_bytes_processed,
		   indexed_at=excluded.indexed_at`,
		repoID, summary.SnapshotID, shortID, firstNonEmpty(summary.BackupStart, time.Now().Format(time.RFC3339Nano)),
		summary.Raw, summary.BackupStart, summary.BackupEnd, summary.FilesNew, summary.FilesChanged,
		summary.FilesUnmodified, summary.DirsNew, summary.DirsChanged, summary.DirsUnmodified,
		summary.DataBlobs, summary.TreeBlobs, summary.DataAdded, summary.DataAddedPacked,
		summary.TotalFilesProcessed, summary.TotalBytesProcessed, summary.Raw, time.Now(), generation,
	)
	return err
}

func (s *RepoCacheService) ListSnapshotFiles(repoID int64, snapshotID, path string, refresh bool) (SnapshotFilesPage, error) {
	path = normalizeSnapshotPath(path)
	queued := false

	indexState, ok, err := s.getFileIndexState(repoID, snapshotID, path)
	if err != nil && !isNoSuchTableError(err) {
		return SnapshotFilesPage{}, err
	}
	if !ok || refresh || indexState.Status == syncStatusStale || indexState.Status == syncStatusFailed || indexState.Status == syncStatusInterrupted {
		if path == "" {
			if err := s.storeSnapshotFileRootsWithNewGeneration(repoID, snapshotID); err != nil {
				return SnapshotFilesPage{}, err
			}
		} else {
			s.EnsureSnapshotPathIndex(repoID, snapshotID, path, refresh)
			queued = true
		}
		indexState, ok, err = s.getFileIndexState(repoID, snapshotID, path)
		if err != nil && !isNoSuchTableError(err) {
			return SnapshotFilesPage{}, err
		}
	}

	items := []FileEntry{}
	total := 0
	if ok && indexState.Generation > 0 {
		rows, err := s.db.Query(
			`SELECT name, path, type, size, mode, mtime
			 FROM repository_snapshot_files
			 WHERE repo_id=? AND snapshot_id=? AND parent_path=? AND generation=?
			 ORDER BY CASE WHEN type='dir' THEN 0 ELSE 1 END, name ASC`,
			repoID, snapshotID, path, indexState.Generation,
		)
		if err != nil {
			return SnapshotFilesPage{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var item FileEntry
			var mode string
			if err := rows.Scan(&item.Name, &item.Path, &item.Type, &item.Size, &mode, &item.ModTime); err != nil {
				return SnapshotFilesPage{}, err
			}
			if strings.TrimSpace(mode) != "" {
				item.Mode = json.RawMessage([]byte(mode))
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return SnapshotFilesPage{}, err
		}
		total = indexState.EntryCount
	}

	return SnapshotFilesPage{
		Items:     items,
		Indexing:  queued || (ok && indexState.Status == syncStatusRunning),
		Stale:     !ok || indexState.Stale || indexState.Status == syncStatusFailed || indexState.Status == syncStatusInterrupted,
		Error:     indexState.LastError,
		IndexedAt: indexState.IndexedAt,
		Total:     total,
	}, nil
}

func (s *RepoCacheService) GetStatsView(repoID int64, refresh bool) (RepoStatsView, error) {
	if refresh {
		_, _ = s.QueueStatsSync(repoID)
	}
	state, _, err := s.getSyncState(repoID, syncDomainStats)
	if err != nil && !isNoSuchTableError(err) {
		return RepoStatsView{}, err
	}
	var payload string
	if state.Generation > 0 {
		_ = s.db.QueryRow(
			`SELECT raw_json FROM repository_stats WHERE repo_id=? AND generation=?`,
			repoID, state.Generation,
		).Scan(&payload)
	}
	return RepoStatsView{
		Data:      json.RawMessage(payload),
		Indexing:  state.Status == syncStatusRunning,
		Stale:     state.Status == syncStatusFailed || state.Status == syncStatusStale || state.Status == syncStatusInterrupted || state.Generation == 0,
		Error:     state.LastError,
		IndexedAt: state.LastSuccessAt,
	}, nil
}

func (s *RepoCacheService) GetKeysView(repoID int64, refresh bool) (RepoKeysView, error) {
	if refresh {
		_, _ = s.QueueKeysSync(repoID)
	}
	state, _, err := s.getSyncState(repoID, syncDomainKeys)
	if err != nil && !isNoSuchTableError(err) {
		return RepoKeysView{}, err
	}

	items := []json.RawMessage{}
	if state.Generation > 0 {
		rows, err := s.db.Query(
			`SELECT raw_json FROM repository_keys
			 WHERE repo_id=? AND generation=?
			 ORDER BY current DESC, created ASC, key_id ASC`,
			repoID, state.Generation,
		)
		if err != nil {
			return RepoKeysView{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				return RepoKeysView{}, err
			}
			items = append(items, json.RawMessage(raw))
		}
		if err := rows.Err(); err != nil {
			return RepoKeysView{}, err
		}
	}

	return RepoKeysView{
		Items:     items,
		Indexing:  state.Status == syncStatusRunning,
		Stale:     state.Status == syncStatusFailed || state.Status == syncStatusStale || state.Status == syncStatusInterrupted || state.Generation == 0,
		Error:     state.LastError,
		IndexedAt: state.LastSuccessAt,
		Total:     len(items),
	}, nil
}

func (s *RepoCacheService) GetSyncStates(repoID int64) ([]SyncState, error) {
	rows, err := s.db.Query(
		`SELECT repo_id, domain, status, phase, progress, generation, last_success_at, last_error, log_id, updated_at
		 FROM repository_sync_state
		 WHERE repo_id=?
		 ORDER BY CASE domain
		 	WHEN 'core' THEN 1
		 	WHEN 'files' THEN 2
		 	WHEN 'stats' THEN 3
		 	WHEN 'keys' THEN 4
		 	WHEN 'check' THEN 5
		 	ELSE 99
		 END`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SyncState
	for rows.Next() {
		state, err := scanSyncState(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, state)
	}
	return items, rows.Err()
}

func (s *RepoCacheService) GetSyncStateDomains(repoID int64) (map[string]SyncStateView, error) {
	states, err := s.GetSyncStates(repoID)
	if err != nil {
		return nil, err
	}

	domains := make(map[string]SyncStateView, len(states)+1)
	for _, state := range states {
		domains[state.Domain] = syncStateToView(state)
	}
	if len(states) > 0 {
		domains["all"] = aggregateSyncState(states)
	}
	return domains, nil
}

func (s *RepoCacheService) EnrichRepository(repo *model.Repository) error {
	if repo == nil {
		return nil
	}
	core, _, err := s.getSyncState(repo.ID, syncDomainCore)
	if err != nil && !isNoSuchTableError(err) {
		return err
	}
	files, _, err := s.getSyncState(repo.ID, syncDomainFiles)
	if err != nil && !isNoSuchTableError(err) {
		return err
	}
	check, _, err := s.getSyncState(repo.ID, syncDomainCheck)
	if err != nil && !isNoSuchTableError(err) {
		return err
	}

	repo.SyncStatus = stateOrDefault(core.Status, syncStatusStale)
	repo.LastSyncedAt = core.LastSuccessAt
	repo.FileIndexStatus = stateOrDefault(files.Status, syncStatusStale)
	repo.LastCheckStatus = check.Status
	repo.LastCheckAt = latestTime(check.LastSuccessAt, timePtr(check.UpdatedAt))

	if core.Generation > 0 {
		_ = s.db.QueryRow(
			`SELECT COUNT(*) FROM repository_snapshots WHERE repo_id=? AND generation=?`,
			repo.ID, core.Generation,
		).Scan(&repo.SnapshotCount)
	}
	return nil
}

func (s *RepoCacheService) EnrichRepositories(repos []model.Repository) ([]model.Repository, error) {
	for i := range repos {
		if err := s.EnrichRepository(&repos[i]); err != nil {
			return nil, err
		}
	}
	return repos, nil
}

func (s *RepoCacheService) MarkStale(repoID int64, domains ...string) {
	for _, domain := range domains {
		if domain == syncDomainFiles {
			_ = s.markFilesDomainStale(repoID, "")
			continue
		}
		_ = s.markSyncStateStale(repoID, domain, "")
	}
}

func (s *RepoCacheService) RecordCheckResult(repoID int64, resultErr error, resultExitCode int, logID int64) {
	status := syncStatusSuccess
	errText := ""
	if resultErr != nil || resultExitCode != 0 {
		status = syncStatusFailed
		if resultErr != nil {
			errText = resultErr.Error()
		} else {
			errText = fmt.Sprintf("check exited with code %d", resultExitCode)
		}
	}
	now := time.Now()
	var successAt *time.Time
	if status == syncStatusSuccess {
		successAt = &now
	}
	_ = s.setSyncState(repoID, syncDomainCheck, status, "", 100, 0, successAt, errText, int64Ptr(logID))
}

func (s *RepoCacheService) ResetRepository(repoID int64) error {
	statements := []string{
		`DELETE FROM repository_sync_state WHERE repo_id=?`,
		`DELETE FROM repository_stats WHERE repo_id=?`,
		`DELETE FROM repository_keys WHERE repo_id=?`,
		`DELETE FROM repository_snapshots WHERE repo_id=?`,
		`DELETE FROM repository_snapshot_files WHERE repo_id=?`,
		`DELETE FROM repository_snapshot_file_indexes WHERE repo_id=?`,
		`DELETE FROM repository_restic_config WHERE repo_id=?`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt, repoID); err != nil {
			return err
		}
	}
	return nil
}

func (s *RepoCacheService) markSyncStateStale(repoID int64, domain, message string) error {
	state, ok, err := s.getSyncState(repoID, domain)
	if err != nil && !isNoSuchTableError(err) {
		return err
	}
	generation := int64(0)
	if ok {
		generation = state.Generation
	}
	return s.setSyncState(repoID, domain, syncStatusStale, "", 0, generation, state.LastSuccessAt, message, state.LogID)
}

func (s *RepoCacheService) markFilesDomainStale(repoID int64, message string) error {
	if _, err := s.db.Exec(
		`UPDATE repository_snapshot_file_indexes
		 SET stale=1, status=CASE
		 	WHEN status=? THEN status
		 	ELSE ?
		 END,
		 updated_at=?
		 WHERE repo_id=?`,
		syncStatusRunning, syncStatusStale, time.Now(), repoID,
	); err != nil {
		return err
	}
	return s.markSyncStateStale(repoID, syncDomainFiles, message)
}

func (s *RepoCacheService) finalizeCoreGeneration(repoID, generation int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`DELETE FROM repository_snapshots WHERE repo_id=? AND generation<>?`,
		`DELETE FROM repository_restic_config WHERE repo_id=? AND generation<>?`,
		`DELETE FROM repository_snapshot_files
		 WHERE repo_id=? AND snapshot_id NOT IN (
		 	SELECT snapshot_id FROM repository_snapshots WHERE repo_id=? AND generation=?
		 )`,
		`DELETE FROM repository_snapshot_file_indexes
		 WHERE repo_id=? AND snapshot_id NOT IN (
		 	SELECT snapshot_id FROM repository_snapshots WHERE repo_id=? AND generation=?
		 )`,
	}
	if _, err := tx.Exec(stmts[0], repoID, generation); err != nil {
		return err
	}
	if _, err := tx.Exec(stmts[1], repoID, generation); err != nil {
		return err
	}
	if _, err := tx.Exec(stmts[2], repoID, repoID, generation); err != nil {
		return err
	}
	if _, err := tx.Exec(stmts[3], repoID, repoID, generation); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *RepoCacheService) storeResticConfig(repoID, generation int64, payload string) error {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return err
	}
	version := asInt64(parsed["version"])
	repositoryID := asString(parsed["id"])
	if repositoryID == "" {
		repositoryID = asString(parsed["repository_id"])
	}
	_, err := s.db.Exec(
		`INSERT INTO repository_restic_config
		 (repo_id, generation, repository_id, version, raw_json, indexed_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		repoID, generation, repositoryID, version, payload, time.Now(), time.Now(),
	)
	return err
}

func (s *RepoCacheService) storeStats(repoID, generation int64, payload string) error {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO repository_stats
		 (repo_id, generation, total_size, total_file_count, total_blob_count, snapshot_count, raw_json, indexed_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		repoID, generation,
		asInt64(parsed["total_size"]),
		asInt64(firstNonNil(parsed["total_file_count"], parsed["total_files"])),
		asInt64(firstNonNil(parsed["total_blob_count"], parsed["total_blobs"])),
		asInt64(firstNonNil(parsed["snapshot_count"], parsed["snapshots_count"])),
		payload, time.Now(), time.Now(),
	)
	return err
}

func (s *RepoCacheService) storeKeys(repoID, generation int64, payload string) error {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`INSERT INTO repository_keys
		 (repo_id, generation, key_id, username, hostname, created, expires, current, raw_json, indexed_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	insertBatch := func(batch []json.RawMessage) error {
		now := time.Now()
		for i, raw := range batch {
			var parsed map[string]any
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return err
			}
			keyID := asString(firstNonNil(parsed["id"], parsed["short_id"]))
			if keyID == "" {
				keyID = fmt.Sprintf("generated-%d", i)
			}
			current := 0
			if asBool(firstNonNil(parsed["current"], parsed["is_current"])) {
				current = 1
			}
			if _, err := stmt.Exec(
				repoID,
				generation,
				keyID,
				asString(firstNonNil(parsed["username"], parsed["user"])),
				asString(parsed["hostname"]),
				asString(parsed["created"]),
				asString(parsed["expires"]),
				current,
				string(raw),
				now,
				now,
			); err != nil {
				return err
			}
		}
		return nil
	}

	var batch []json.RawMessage
	if strings.HasPrefix(trimmed, "[") {
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delim, ok := token.(json.Delim); !ok || delim != '[' {
			return fmt.Errorf("expected JSON array of keys")
		}
		for decoder.More() {
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				return err
			}
			batch = append(batch, raw)
			if len(batch) == indexBatchSize {
				if err := insertBatch(batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
	} else {
		scanner := bufio.NewScanner(strings.NewReader(payload))
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			batch = append(batch, json.RawMessage(line))
			if len(batch) == indexBatchSize {
				if err := insertBatch(batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}
	if len(batch) > 0 {
		if err := insertBatch(batch); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *RepoCacheService) replaceSnapshots(repoID, generation int64, payload string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`INSERT INTO repository_snapshots
		 (repo_id, snapshot_id, short_id, time, hostname, username, uid, gid, tags, paths, tree,
		  program_version, summary, backup_start, backup_end, files_new, files_changed,
		  files_unmodified, dirs_new, dirs_changed, dirs_unmodified, data_blobs, tree_blobs,
		  data_added, data_added_packed, total_files_processed, total_bytes_processed,
		  raw_json, indexed_at, generation)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, snapshot_id, generation) DO UPDATE SET
		   short_id=excluded.short_id,
		   time=excluded.time,
		   hostname=excluded.hostname,
		   username=excluded.username,
		   uid=excluded.uid,
		   gid=excluded.gid,
		   tags=excluded.tags,
		   paths=excluded.paths,
		   tree=excluded.tree,
		   program_version=excluded.program_version,
		   summary=excluded.summary,
		   backup_start=excluded.backup_start,
		   backup_end=excluded.backup_end,
		   files_new=excluded.files_new,
		   files_changed=excluded.files_changed,
		   files_unmodified=excluded.files_unmodified,
		   dirs_new=excluded.dirs_new,
		   dirs_changed=excluded.dirs_changed,
		   dirs_unmodified=excluded.dirs_unmodified,
		   data_blobs=excluded.data_blobs,
		   tree_blobs=excluded.tree_blobs,
		   data_added=excluded.data_added,
		   data_added_packed=excluded.data_added_packed,
		   total_files_processed=excluded.total_files_processed,
		   total_bytes_processed=excluded.total_bytes_processed,
		   raw_json=excluded.raw_json,
		   indexed_at=excluded.indexed_at,
		   generation=excluded.generation`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(payload)))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("expected JSON array of snapshots")
	}

	var batch []json.RawMessage
	flush := func() error {
		now := time.Now()
		for _, raw := range batch {
			var snap Snapshot
			if err := json.Unmarshal(raw, &snap); err != nil {
				return err
			}
			if snapshotSummaryIsEmpty(snap) {
				if err := s.fillSnapshotSummaryFromDB(tx, repoID, &snap); err != nil {
					return err
				}
			}
			tagsRaw, _ := json.Marshal(snap.Tags)
			pathsRaw, _ := json.Marshal(snap.Paths)
			summaryRaw := "{}"
			if len(snap.Summary) > 0 {
				summaryRaw = string(snap.Summary)
			}
			if _, err := stmt.Exec(
				repoID, snap.ID, snap.ShortID, snap.Time, snap.Hostname, snap.Username, snap.UID, snap.GID,
				string(tagsRaw), string(pathsRaw), snap.Tree, snap.ProgramVersion, summaryRaw,
				snap.BackupStart, snap.BackupEnd, snap.FilesNew, snap.FilesChanged, snap.FilesUnmodified,
				snap.DirsNew, snap.DirsChanged, snap.DirsUnmodified, snap.DataBlobs, snap.TreeBlobs,
				snap.DataAdded, snap.DataAddedPacked, snap.TotalFilesProcessed, snap.TotalBytesProcessed,
				string(raw), now, generation,
			); err != nil {
				return err
			}
		}
		batch = batch[:0]
		return nil
	}

	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		batch = append(batch, raw)
		if len(batch) == indexBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if len(batch) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	return tx.Commit()
}

func snapshotSummaryIsEmpty(snap Snapshot) bool {
	return len(snap.Summary) == 0 &&
		snap.BackupStart == "" &&
		snap.BackupEnd == "" &&
		snap.FilesNew == 0 &&
		snap.FilesChanged == 0 &&
		snap.FilesUnmodified == 0 &&
		snap.DirsNew == 0 &&
		snap.DirsChanged == 0 &&
		snap.DirsUnmodified == 0 &&
		snap.DataBlobs == 0 &&
		snap.TreeBlobs == 0 &&
		snap.DataAdded == 0 &&
		snap.DataAddedPacked == 0 &&
		snap.TotalFilesProcessed == 0 &&
		snap.TotalBytesProcessed == 0
}

func (s *RepoCacheService) fillSnapshotSummaryFromDB(tx *sql.Tx, repoID int64, snap *Snapshot) error {
	var summaryRaw string
	err := tx.QueryRow(
		`SELECT summary, backup_start, backup_end, files_new, files_changed, files_unmodified,
		        dirs_new, dirs_changed, dirs_unmodified, data_blobs, tree_blobs, data_added,
		        data_added_packed, total_files_processed, total_bytes_processed
		 FROM repository_snapshots
		 WHERE repo_id=? AND snapshot_id=? AND (
		   summary<>'{}' OR total_files_processed<>0 OR total_bytes_processed<>0 OR data_added<>0 OR data_added_packed<>0
		 )
		 ORDER BY generation DESC
		 LIMIT 1`,
		repoID, snap.ID,
	).Scan(
		&summaryRaw, &snap.BackupStart, &snap.BackupEnd, &snap.FilesNew, &snap.FilesChanged, &snap.FilesUnmodified,
		&snap.DirsNew, &snap.DirsChanged, &snap.DirsUnmodified, &snap.DataBlobs, &snap.TreeBlobs, &snap.DataAdded,
		&snap.DataAddedPacked, &snap.TotalFilesProcessed, &snap.TotalBytesProcessed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(summaryRaw) != "" && summaryRaw != "{}" {
		snap.Summary = json.RawMessage(summaryRaw)
	}
	return nil
}

func (s *RepoCacheService) storeSnapshotFiles(repoID int64, snapshotID, parentPath string, generation int64, payload string) (int, error) {
	parentPath = normalizeSnapshotPath(parentPath)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM repository_snapshot_files
		 WHERE repo_id=? AND snapshot_id=? AND parent_path=? AND generation<>?`,
		repoID, snapshotID, parentPath, generation,
	); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		`DELETE FROM repository_snapshot_files
		 WHERE repo_id=? AND snapshot_id=? AND parent_path=? AND generation=?`,
		repoID, snapshotID, parentPath, generation,
	); err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO repository_snapshot_files
		 (repo_id, snapshot_id, path, parent_path, name, type, size, mode, mtime, raw_json, generation, indexed_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	insertBatch := func(batch []json.RawMessage) error {
		now := time.Now()
		for _, raw := range batch {
			var entry FileEntry
			if err := json.Unmarshal(raw, &entry); err != nil {
				return err
			}
			entry.Path = normalizeSnapshotPath(entry.Path)
			if entry.Type == "" || entry.Path == "" || entry.Path == parentPath || parentSnapshotPath(entry.Path) != parentPath {
				continue
			}
			if entry.Name == "" {
				entry.Name = snapshotPathBase(entry.Path)
			}
			mode := strings.TrimSpace(string(entry.Mode))
			if mode == "" || mode == "null" {
				mode = ""
			}
			if _, err := stmt.Exec(
				repoID,
				snapshotID,
				entry.Path,
				parentPath,
				entry.Name,
				entry.Type,
				entry.Size,
				mode,
				entry.ModTime,
				string(raw),
				generation,
				now,
				now,
			); err != nil {
				return err
			}
			count++
		}
		return nil
	}

	trimmed := strings.TrimSpace(payload)
	var batch []json.RawMessage
	if strings.HasPrefix(trimmed, "[") {
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		token, err := decoder.Token()
		if err != nil {
			return 0, err
		}
		if delim, ok := token.(json.Delim); !ok || delim != '[' {
			return 0, fmt.Errorf("expected JSON array from restic ls")
		}
		for decoder.More() {
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				return 0, err
			}
			batch = append(batch, raw)
			if len(batch) == indexBatchSize {
				if err := insertBatch(batch); err != nil {
					return 0, err
				}
				batch = batch[:0]
			}
		}
		if _, err := decoder.Token(); err != nil {
			return 0, err
		}
	} else {
		scanner := bufio.NewScanner(strings.NewReader(payload))
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			batch = append(batch, json.RawMessage(line))
			if len(batch) == indexBatchSize {
				if err := insertBatch(batch); err != nil {
					return 0, err
				}
				batch = batch[:0]
			}
		}
		if err := scanner.Err(); err != nil {
			return 0, err
		}
	}
	if len(batch) > 0 {
		if err := insertBatch(batch); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *RepoCacheService) storeSnapshotFileRootsWithNewGeneration(repoID int64, snapshotID string) error {
	generation, err := s.nextGeneration(repoID, syncDomainFiles)
	if err != nil {
		return err
	}
	count, err := s.storeSnapshotFileRoots(repoID, snapshotID, generation)
	if err != nil {
		return err
	}
	now := time.Now()
	return s.setFileIndexState(repoID, snapshotID, "", syncStatusSuccess, count, &now, "", false, generation)
}

func (s *RepoCacheService) storeSnapshotFileRoots(repoID int64, snapshotID string, generation int64) (int, error) {
	paths, err := s.snapshotPaths(repoID, snapshotID)
	if err != nil {
		return 0, err
	}
	if len(paths) == 0 {
		if err := s.setFileIndexState(repoID, snapshotID, "", syncStatusStale, 0, nil, "snapshot paths are not indexed yet", true, 0); err != nil {
			return 0, err
		}
		return 0, nil
	}
	entries := rootEntriesFromSnapshotPaths(paths)
	if len(entries) == 0 {
		if err := s.setFileIndexState(repoID, snapshotID, "", syncStatusSuccess, 0, timePtr(time.Now()), "", false, generation); err != nil {
			return 0, err
		}
		return 0, nil
	}

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			return 0, err
		}
		lines = append(lines, string(raw))
	}
	return s.storeSnapshotFiles(repoID, snapshotID, "", generation, strings.Join(lines, "\n"))
}

func (s *RepoCacheService) snapshotPaths(repoID int64, snapshotID string) ([]string, error) {
	state, ok, err := s.getSyncState(repoID, syncDomainCore)
	if err != nil && !isNoSuchTableError(err) {
		return nil, err
	}

	var raw string
	if ok && state.Generation > 0 {
		err = s.db.QueryRow(
			`SELECT paths FROM repository_snapshots WHERE repo_id=? AND snapshot_id=? AND generation=?`,
			repoID, snapshotID, state.Generation,
		).Scan(&raw)
	} else {
		err = s.db.QueryRow(
			`SELECT paths FROM repository_snapshots WHERE repo_id=? AND snapshot_id=? ORDER BY generation DESC LIMIT 1`,
			repoID, snapshotID,
		).Scan(&raw)
	}
	if errors.Is(err, sql.ErrNoRows) || isNoSuchTableError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil, err
	}
	return paths, nil
}

func (s *RepoCacheService) nextGeneration(repoID int64, domain string) (int64, error) {
	current, err := s.currentGeneration(repoID, domain)
	if err != nil {
		return 0, err
	}
	return current + 1, nil
}

func (s *RepoCacheService) currentGeneration(repoID int64, domain string) (int64, error) {
	state, ok, err := s.getSyncState(repoID, domain)
	if err != nil {
		if isNoSuchTableError(err) {
			return 0, nil
		}
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	return state.Generation, nil
}

func (s *RepoCacheService) getSyncState(repoID int64, domain string) (SyncState, bool, error) {
	row := s.db.QueryRow(
		`SELECT repo_id, domain, status, phase, progress, generation, last_success_at, last_error, log_id, updated_at
		 FROM repository_sync_state WHERE repo_id=? AND domain=?`,
		repoID, domain,
	)
	state, err := scanSyncState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncState{}, false, nil
	}
	if err != nil {
		return SyncState{}, false, err
	}
	return state, true, nil
}

func (s *RepoCacheService) setSyncState(repoID int64, domain, status, phase string, progress int, generation int64, lastSuccessAt *time.Time, lastError string, logID *int64) error {
	logID = sanitizeLogID(logID)
	_, err := s.db.Exec(
		`INSERT INTO repository_sync_state
		 (repo_id, domain, status, phase, progress, generation, last_success_at, last_error, log_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, domain) DO UPDATE SET
		 	status=excluded.status,
		 	phase=excluded.phase,
		 	progress=excluded.progress,
		 	generation=CASE WHEN excluded.generation = 0 THEN repository_sync_state.generation ELSE excluded.generation END,
		 	last_success_at=COALESCE(excluded.last_success_at, repository_sync_state.last_success_at),
		 	last_error=excluded.last_error,
		 	log_id=COALESCE(excluded.log_id, repository_sync_state.log_id),
		 	updated_at=excluded.updated_at`,
		repoID,
		domain,
		status,
		phase,
		progress,
		generation,
		lastSuccessAt,
		lastError,
		logID,
		time.Now(),
		time.Now(),
	)
	return err
}

type fileIndexState struct {
	Status     string
	EntryCount int
	IndexedAt  *time.Time
	LastError  string
	Stale      bool
	Generation int64
}

func (s *RepoCacheService) getFileIndexState(repoID int64, snapshotID, path string) (fileIndexState, bool, error) {
	var state fileIndexState
	var indexedAt sql.NullTime
	var stale int
	err := s.db.QueryRow(
		`SELECT status, entry_count, indexed_at, last_error, stale, generation
		 FROM repository_snapshot_file_indexes
		 WHERE repo_id=? AND snapshot_id=? AND path=?`,
		repoID, snapshotID, path,
	).Scan(&state.Status, &state.EntryCount, &indexedAt, &state.LastError, &stale, &state.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return fileIndexState{}, false, nil
	}
	if err != nil {
		return fileIndexState{}, false, err
	}
	if indexedAt.Valid {
		state.IndexedAt = &indexedAt.Time
	}
	state.Stale = stale == 1
	return state, true, nil
}

func (s *RepoCacheService) setFileIndexState(repoID int64, snapshotID, path, status string, entryCount int, indexedAt *time.Time, lastError string, stale bool, generation int64) error {
	staleInt := 0
	if stale {
		staleInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO repository_snapshot_file_indexes
		 (repo_id, snapshot_id, path, status, entry_count, indexed_at, last_error, stale, generation, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, snapshot_id, path) DO UPDATE SET
		 	status=excluded.status,
		 	entry_count=excluded.entry_count,
		 	indexed_at=excluded.indexed_at,
		 	last_error=excluded.last_error,
		 	stale=excluded.stale,
		 	generation=CASE WHEN excluded.generation = 0 THEN repository_snapshot_file_indexes.generation ELSE excluded.generation END,
		 	updated_at=excluded.updated_at`,
		repoID, snapshotID, path, status, entryCount, indexedAt, lastError, staleInt, generation, time.Now(), time.Now(),
	)
	return err
}

type syncStateScanner interface {
	Scan(dest ...any) error
}

func scanSyncState(scanner syncStateScanner) (SyncState, error) {
	var state SyncState
	var lastSuccess sql.NullTime
	var logID sql.NullInt64
	err := scanner.Scan(
		&state.RepoID,
		&state.Domain,
		&state.Status,
		&state.Phase,
		&state.Progress,
		&state.Generation,
		&lastSuccess,
		&state.LastError,
		&logID,
		&state.UpdatedAt,
	)
	if err != nil {
		return SyncState{}, err
	}
	if lastSuccess.Valid {
		state.LastSuccessAt = &lastSuccess.Time
	}
	if logID.Valid {
		state.LogID = &logID.Int64
	}
	return state, nil
}

func normalizeSnapshotPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = pathpkg.Clean(path)
	if path == "." || path == "/" {
		return ""
	}
	return path
}

func parentSnapshotPath(path string) string {
	path = normalizeSnapshotPath(path)
	if path == "" {
		return ""
	}
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return ""
	}
	return path[:idx]
}

func snapshotPathBase(path string) string {
	path = normalizeSnapshotPath(path)
	if path == "" {
		return "/"
	}
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

func rootEntriesFromSnapshotPaths(paths []string) []FileEntry {
	roots := map[string]FileEntry{}
	for _, snapshotPath := range paths {
		normalized := normalizeSnapshotPath(snapshotPath)
		if normalized == "" {
			continue
		}
		trimmed := strings.TrimPrefix(normalized, "/")
		firstSegment := trimmed
		if idx := strings.Index(trimmed, "/"); idx >= 0 {
			firstSegment = trimmed[:idx]
		}
		if firstSegment == "" {
			continue
		}
		rootPath := "/" + firstSegment
		roots[rootPath] = FileEntry{
			Name: snapshotPathBase(rootPath),
			Path: rootPath,
			Type: "dir",
		}
	}

	keys := make([]string, 0, len(roots))
	for key := range roots {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entries := make([]FileEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, roots[key])
	}
	return entries
}

func syncStateToView(state SyncState) SyncStateView {
	return SyncStateView{
		Status:       state.Status,
		Phase:        state.Phase,
		Progress:     state.Progress,
		Generation:   state.Generation,
		LastSyncedAt: state.LastSuccessAt,
		Error:        state.LastError,
		LogID:        state.LogID,
		UpdatedAt:    state.UpdatedAt,
	}
}

func aggregateSyncState(states []SyncState) SyncStateView {
	view := SyncStateView{Status: syncStatusStale}
	if len(states) == 0 {
		return view
	}

	var latestUpdated *time.Time
	var latestSuccess *time.Time
	errors := make([]string, 0, len(states))
	hasRunning := false
	hasSuccess := false
	hasStale := false
	hasFailure := false
	var coreState *SyncState
	var activeLogID *int64

	for _, state := range states {
		state := state
		if state.Domain == syncDomainCore {
			coreState = &state
		}
		if latestUpdated == nil || state.UpdatedAt.After(*latestUpdated) {
			view.Phase = state.Phase
			view.Progress = state.Progress
			view.Generation = maxInt64(view.Generation, state.Generation)
			view.LogID = state.LogID
			view.UpdatedAt = state.UpdatedAt
			latestUpdated = &view.UpdatedAt
		}
		if state.Status == syncStatusSuccess && state.LastSuccessAt != nil && (latestSuccess == nil || state.LastSuccessAt.After(*latestSuccess)) {
			view.LastSyncedAt = state.LastSuccessAt
			latestSuccess = state.LastSuccessAt
		}
		if trimmed := strings.TrimSpace(state.LastError); trimmed != "" {
			errors = append(errors, state.Domain+": "+trimmed)
		}
		if state.Status == syncStatusRunning && state.LogID != nil {
			activeLogID = state.LogID
		}

		switch state.Status {
		case syncStatusRunning:
			hasRunning = true
		case syncStatusFailed, syncStatusInterrupted:
			hasFailure = true
		case syncStatusStale:
			hasStale = true
		case syncStatusSuccess:
			hasSuccess = true
		}
	}

	switch {
	case coreState != nil && coreState.Status == syncStatusRunning:
		view.Status = syncStatusRunning
		view.LogID = coreState.LogID
		view.Error = ""
	case coreState != nil && (coreState.Status == syncStatusFailed || coreState.Status == syncStatusInterrupted):
		view.Status = syncStatusFailed
	case coreState != nil && coreState.Status == syncStatusStale:
		view.Status = syncStatusStale
	case coreState != nil && coreState.Status == syncStatusSuccess:
		switch {
		case hasRunning:
			view.Status = syncStatusRunning
			view.LogID = activeLogID
			view.Error = ""
		case hasFailure || hasStale:
			view.Status = syncStatusPartial
		default:
			view.Status = syncStatusSuccess
		}
	case hasRunning:
		view.Status = syncStatusRunning
		view.LogID = activeLogID
		view.Error = ""
	case hasFailure:
		view.Status = syncStatusFailed
	case hasStale:
		view.Status = syncStatusStale
	case hasSuccess:
		view.Status = syncStatusSuccess
	default:
		view.Status = syncStatusStale
	}

	if len(errors) > 0 {
		view.Error = strings.Join(errors, "; ")
	}
	return view
}

func maxInt64(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}

func syncKey(repoID int64, domain string) string {
	return strconv.FormatInt(repoID, 10) + ":" + domain
}

func stateOrDefault(status, fallback string) string {
	if strings.TrimSpace(status) == "" {
		return fallback
	}
	return status
}

func latestTime(a, b *time.Time) *time.Time {
	if a != nil && a.IsZero() {
		a = nil
	}
	if b != nil && b.IsZero() {
		b = nil
	}
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if b.After(*a) {
		return b
	}
	return a
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func sanitizeLogID(logID *int64) *int64 {
	if logID == nil || *logID <= 0 {
		return nil
	}
	return logID
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case fmt.Stringer:
		return value.String()
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(value, 10)
	case int:
		return strconv.Itoa(value)
	default:
		return ""
	}
}

func asInt64(v any) int64 {
	switch value := v.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		n, _ := value.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return n
	default:
		return 0
	}
}

func asBool(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

type backupSummaryPayload struct {
	MessageType         string          `json:"message_type"`
	SnapshotID          string          `json:"snapshot_id"`
	BackupStart         string          `json:"backup_start"`
	BackupEnd           string          `json:"backup_end"`
	FilesNew            int64           `json:"files_new"`
	FilesChanged        int64           `json:"files_changed"`
	FilesUnmodified     int64           `json:"files_unmodified"`
	DirsNew             int64           `json:"dirs_new"`
	DirsChanged         int64           `json:"dirs_changed"`
	DirsUnmodified      int64           `json:"dirs_unmodified"`
	DataBlobs           int64           `json:"data_blobs"`
	TreeBlobs           int64           `json:"tree_blobs"`
	DataAdded           int64           `json:"data_added"`
	DataAddedPacked     int64           `json:"data_added_packed"`
	TotalFilesProcessed int64           `json:"total_files_processed"`
	TotalBytesProcessed int64           `json:"total_bytes_processed"`
	Raw                 json.RawMessage `json:"-"`
}

func parseBackupSummaryPayload(payload string) (backupSummaryPayload, bool, error) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return backupSummaryPayload{}, false, nil
	}
	var summary backupSummaryPayload
	parseRaw := func(raw json.RawMessage) (bool, error) {
		var item backupSummaryPayload
		if err := json.Unmarshal(raw, &item); err != nil {
			return false, err
		}
		if item.MessageType != "summary" || item.SnapshotID == "" {
			return false, nil
		}
		item.Raw = append(json.RawMessage(nil), raw...)
		summary = item
		return true, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
			return backupSummaryPayload{}, false, err
		}
		for _, raw := range items {
			if ok, err := parseRaw(raw); ok || err != nil {
				return summary, ok, err
			}
		}
		return backupSummaryPayload{}, false, nil
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		if ok, err := parseRaw(json.RawMessage(line)); ok || err != nil {
			return summary, ok, err
		}
	}
	if err := scanner.Err(); err != nil {
		return backupSummaryPayload{}, false, err
	}
	return backupSummaryPayload{}, false, nil
}

func isNoSuchTableError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}

func commandPayload(result executor.ExecResult, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout, errors.New(strings.TrimSpace(result.Stderr + "\n" + result.Stdout))
	}
	return result.Stdout, nil
}
