// backend/internal/service/repo_service.go
package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/repository"
	"github.com/autorestic/autorestic/internal/ws"
)

const (
	defaultPruneCronExpr = "0 3 * * 0"
	defaultPruneArgs     = "[]"
	defaultCheckCronExpr = "0 4 1 * *"
	defaultCheckArgs     = `["--read-data-subset=10%"]`
)

var ErrRepositoryAccessInvalid = errors.New("repository exists but credentials or access permissions are invalid")
var ErrRepositoryAlreadyExists = errors.New("repository name or endpoint already exists")
var ErrRepositoryNotFound = errors.New("repository does not exist or is not accessible with the supplied settings")
var ErrRepositoryLocked = errors.New("repository is locked; confirm automatic unlock before importing, or retry after other restic processes finish")

type CreateRepoResult struct {
	ID           int64
	ImportStatus string
	ImportLogID  int64
	InitStatus   string
	InitLogID    int64
	UnlockLogID  int64
}

type RepositoryProbe struct {
	Exists     bool
	Accessible bool
	Locked     bool
}

type RepoService struct {
	store    *repository.RepoStore
	executor *executor.Executor
	encKey   []byte
	hub      *ws.Hub
	cache    *RepoCacheService

	repoOpMu sync.Mutex
	repoOps  map[int64]*repoOperationState
}

type repoOperationState struct {
	class repoOperationClass
	kind  string
	done  chan struct{}
}

type repoOperationReservation struct {
	repoID int64
	state  *repoOperationState
}

type resticLockInfo struct {
	ID        string `json:"id"`
	Time      string `json:"time"`
	Exclusive bool   `json:"exclusive"`
	Hostname  string `json:"hostname"`
	Username  string `json:"username"`
	PID       int    `json:"pid"`
	UID       int    `json:"uid"`
	GID       int    `json:"gid"`
}

type resticLockOwnerStatus struct {
	CurrentInstance bool
	Alive           bool
}

type repoOperationClass string

const (
	repoOperationRead      repoOperationClass = "read"
	repoOperationExclusive repoOperationClass = "exclusive"
)

func NewRepoService(store *repository.RepoStore, exec *executor.Executor, encKeyPath string, hub *ws.Hub) (*RepoService, error) {
	key, err := loadOrCreateKey(encKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load encryption key: %w", err)
	}
	svc := &RepoService{
		store:    store,
		executor: exec,
		encKey:   key,
		hub:      hub,
		repoOps:  map[int64]*repoOperationState{},
	}
	svc.cache = NewRepoCacheService(store.DB(), svc)
	return svc, nil
}

func (s *RepoService) Cache() *RepoCacheService {
	return s.cache
}

func loadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		trimmed := strings.TrimSpace(string(data))
		if len(trimmed) != 64 {
			return nil, fmt.Errorf("invalid encryption key length in %s", path)
		}
		key, decodeErr := hex.DecodeString(trimmed)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode encryption key %s: %w", path, decodeErr)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid encryption key size in %s", path)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *RepoService) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

func (s *RepoService) decrypt(cipherHex string) (string, error) {
	data, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *RepoService) List() ([]model.Repository, error) {
	repos, err := s.store.List()
	if err != nil {
		return nil, err
	}
	return s.cache.EnrichRepositories(repos)
}

func (s *RepoService) GetByID(id int64) (*model.Repository, error) {
	repo, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.cache.EnrichRepository(repo); err != nil {
		return nil, err
	}
	return repo, nil
}

func (s *RepoService) Create(req model.CreateRepoRequest) (CreateRepoResult, error) {
	endpoint, err := normalizeRepoEndpoint(req)
	if err != nil {
		return CreateRepoResult{}, err
	}
	if err := s.ensureRepoUnique(req.Name, req.Type, endpoint, 0); err != nil {
		return CreateRepoResult{}, err
	}
	accessReq := model.RepositoryAccessRequest{
		Type:           req.Type,
		Endpoint:       endpoint,
		Password:       req.Password,
		RcloneConfig:   req.RcloneConfig,
		WebdavURL:      req.WebdavURL,
		WebdavUser:     req.WebdavUser,
		WebdavPassword: req.WebdavPassword,
	}
	probe, err := s.ProbeRepositoryAccess(context.Background(), accessReq)
	if err != nil {
		return CreateRepoResult{}, err
	}
	var unlockLogID int64
	if probe.Exists && probe.Accessible && probe.Locked && req.AutoUnlock {
		unlockLogID, err = s.UnlockRepositoryAccess(context.Background(), accessReq)
		if err != nil {
			return CreateRepoResult{}, err
		}
		probe, err = s.ProbeRepositoryAccess(context.Background(), accessReq)
		if err != nil {
			return CreateRepoResult{}, err
		}
		if probe.Locked {
			return CreateRepoResult{}, ErrRepositoryLocked
		}
	}
	encPwd, err := s.encrypt(req.Password)
	if err != nil {
		return CreateRepoResult{}, fmt.Errorf("encrypt password: %w", err)
	}

	var encWebdavPwd string
	if req.WebdavPassword != "" {
		encWebdavPwd, err = s.encrypt(req.WebdavPassword)
		if err != nil {
			return CreateRepoResult{}, err
		}
	}

	repo := &model.Repository{
		Name:                    req.Name,
		Type:                    req.Type,
		Endpoint:                endpoint,
		PasswordEncrypted:       encPwd,
		WebdavURL:               req.WebdavURL,
		WebdavUser:              req.WebdavUser,
		WebdavPasswordEncrypted: encWebdavPwd,
		Options:                 req.Options,
		PruneEnabled:            boolValue(req.PruneEnabled, true),
		PruneCronExpr:           stringValue(req.PruneCronExpr, defaultPruneCronExpr),
		PruneArgs:               stringValue(req.PruneArgs, defaultPruneArgs),
		CheckEnabled:            boolValue(req.CheckEnabled, true),
		CheckCronExpr:           stringValue(req.CheckCronExpr, defaultCheckCronExpr),
		CheckArgs:               stringValue(req.CheckArgs, defaultCheckArgs),
	}
	if repo.Options == "" {
		repo.Options = "{}"
	}
	if req.RcloneConfig != "" {
		repo.RcloneConfigEncrypted, err = s.encrypt(req.RcloneConfig)
		if err != nil {
			return CreateRepoResult{}, fmt.Errorf("encrypt rclone config: %w", err)
		}
	}

	id, err := s.store.Create(repo)
	if err != nil {
		if isUniqueConstraintError(err) {
			return CreateRepoResult{}, ErrRepositoryAlreadyExists
		}
		return CreateRepoResult{}, err
	}

	result := CreateRepoResult{ID: id, UnlockLogID: unlockLogID}
	if probe.Exists && probe.Accessible {
		logID, syncErr := s.cache.QueueInitialImport(id)
		if syncErr != nil {
			return result, syncErr
		}
		result.ImportStatus = syncStatusRunning
		result.ImportLogID = logID
		return result, nil
	}

	if req.InitOnCreate {
		logID, initErr := s.InitRepoAsync(id)
		if initErr != nil {
			result.InitStatus = "failed_to_start"
			return result, nil
		}
		result.InitStatus = syncStatusRunning
		result.InitLogID = logID
	}
	return result, nil
}

func (s *RepoService) ValidateRepoAccess(req model.RepositoryAccessRequest) error {
	probe, err := s.ProbeRepositoryAccess(context.Background(), req)
	if err != nil {
		return err
	}
	if !probe.Exists || !probe.Accessible {
		return ErrRepositoryNotFound
	}
	return nil
}

func (s *RepoService) ensureRepoUnique(name, repoType, endpoint string, currentID int64) error {
	repos, err := s.store.List()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	endpoint = strings.TrimSpace(endpoint)
	for _, repo := range repos {
		if currentID != 0 && repo.ID == currentID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(repo.Name), name) ||
			(repo.Type == repoType && strings.TrimSpace(repo.Endpoint) == endpoint) {
			return ErrRepositoryAlreadyExists
		}
	}
	return nil
}

func (s *RepoService) ProbeRepositoryAccess(ctx context.Context, req model.RepositoryAccessRequest) (RepositoryProbe, error) {
	endpoint, err := normalizeProbeEndpoint(req)
	if err != nil {
		return RepositoryProbe{}, err
	}
	env, cleanup, err := buildAccessProbeEnv(req, endpoint)
	if err != nil {
		return RepositoryProbe{}, err
	}
	defer cleanup()

	result := s.executor.Run(ctx, executor.ExecRequest{
		Trigger: "system_query",
		Args:    []string{"cat", "config"},
		Env:     env,
		Hub:     s.hub,
	})
	output := result.Stdout + "\n" + result.Stderr + "\n" + result.CombinedOutput
	if result.ExitCode == 0 {
		locked, _ := s.detectRepositoryLocks(ctx, env)
		return RepositoryProbe{Exists: true, Accessible: true, Locked: locked}, nil
	}
	localResticLayout := req.Type == "local" && localPathLooksLikeResticRepository(endpoint)
	if isCredentialOrAccessError(output) {
		return RepositoryProbe{Exists: true, Accessible: false}, ErrRepositoryAccessInvalid
	}
	if isRepoMissingError(output) {
		if localResticLayout {
			return RepositoryProbe{Exists: true, Accessible: false}, ErrRepositoryAccessInvalid
		}
		return RepositoryProbe{Exists: false, Accessible: false}, nil
	}
	if localResticLayout {
		return RepositoryProbe{Exists: true, Accessible: false}, ErrRepositoryAccessInvalid
	}
	return RepositoryProbe{Exists: false, Accessible: false}, nil
}

func (s *RepoService) detectRepositoryLocks(ctx context.Context, env []string) (bool, error) {
	result := s.executor.Run(ctx, executor.ExecRequest{
		Trigger: "system_query",
		Args:    []string{"list", "locks", "--no-lock"},
		Env:     env,
		Hub:     s.hub,
	})
	output := result.Stdout + "\n" + result.Stderr + "\n" + result.CombinedOutput
	if result.ExitCode == 0 {
		if strings.TrimSpace(result.Stdout) != "" {
			return true, nil
		}
		return hasResticLockIDs(result.CombinedOutput), nil
	}
	if isLockError(output) || result.ExitCode == 11 {
		return true, nil
	}
	return false, result.Err
}

func (s *RepoService) UnlockRepositoryAccess(ctx context.Context, req model.RepositoryAccessRequest) (int64, error) {
	endpoint, err := normalizeProbeEndpoint(req)
	if err != nil {
		return 0, err
	}
	env, cleanup, err := buildAccessProbeEnv(req, endpoint)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	result := s.executor.Run(ctx, executor.ExecRequest{
		Trigger: "manual",
		Args:    []string{"unlock"},
		Env:     env,
		Hub:     s.hub,
	})
	if result.Err != nil {
		return result.LogID, result.Err
	}
	if result.ExitCode != 0 {
		return result.LogID, errors.New(strings.TrimSpace(result.Stderr + "\n" + result.Stdout))
	}
	return result.LogID, nil
}

func localPathLooksLikeResticRepository(endpoint string) bool {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return false
	}
	required := []struct {
		rel   string
		isDir bool
	}{
		{rel: "config", isDir: false},
		{rel: "data", isDir: true},
		{rel: "index", isDir: true},
		{rel: "keys", isDir: true},
	}
	for _, item := range required {
		info, err := os.Stat(filepath.Join(endpoint, item.rel))
		if err != nil {
			return false
		}
		if item.isDir != info.IsDir() {
			return false
		}
	}
	return true
}

func isCredentialOrAccessError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "wrong password") ||
		strings.Contains(lower, "no key found") ||
		strings.Contains(lower, "password is incorrect") ||
		strings.Contains(lower, "invalid password") ||
		strings.Contains(lower, "403 forbidden") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "authorization failed") ||
		strings.Contains(lower, "authentication failed")
}

func isLockError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "already locked") ||
		strings.Contains(lower, "repository is locked") ||
		strings.Contains(lower, "unable to create lock")
}

func hasResticLockIDs(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 8 || len(trimmed) > 64 {
			continue
		}
		if strings.Trim(trimmed, "0123456789abcdef") == "" {
			return true
		}
	}
	return false
}

func isRepoMissingError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "no such file or directory") ||
		strings.Contains(lower, "unable to open config file") ||
		strings.Contains(lower, "404 not found") ||
		strings.Contains(lower, "repository does not exist") ||
		strings.Contains(lower, "not a repository")
}

func boolValue(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func stringValue(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (s *RepoService) Update(id int64, req model.UpdateRepoRequest) error {
	repo, err := s.store.GetByID(id)
	if err != nil {
		return err
	}
	needsResync := req.Endpoint != nil || req.Password != nil || req.RcloneConfig != nil ||
		req.WebdavURL != nil || req.WebdavUser != nil || req.WebdavPassword != nil

	if req.Name != nil {
		repo.Name = *req.Name
	}
	if req.Endpoint != nil {
		repo.Endpoint = strings.TrimSpace(*req.Endpoint)
	}
	if req.Password != nil && *req.Password != "" {
		encPwd, err := s.encrypt(*req.Password)
		if err != nil {
			return fmt.Errorf("encrypt password: %w", err)
		}
		repo.PasswordEncrypted = encPwd
	}
	if req.RcloneConfig != nil {
		if *req.RcloneConfig == "" {
			repo.RcloneConfigEncrypted = ""
		} else {
			encConfig, err := s.encrypt(*req.RcloneConfig)
			if err != nil {
				return fmt.Errorf("encrypt rclone config: %w", err)
			}
			repo.RcloneConfigEncrypted = encConfig
		}
	}
	if req.WebdavURL != nil {
		repo.WebdavURL = strings.TrimSpace(*req.WebdavURL)
		if repo.Type == "webdav" {
			repo.Endpoint = repo.WebdavURL
		}
	}
	if req.WebdavUser != nil {
		repo.WebdavUser = *req.WebdavUser
	}
	if req.WebdavPassword != nil && *req.WebdavPassword != "" {
		encPwd, err := s.encrypt(*req.WebdavPassword)
		if err != nil {
			return fmt.Errorf("encrypt webdav password: %w", err)
		}
		repo.WebdavPasswordEncrypted = encPwd
	}
	if req.Options != nil {
		repo.Options = *req.Options
	}
	if req.PruneEnabled != nil {
		repo.PruneEnabled = *req.PruneEnabled
	}
	if req.PruneCronExpr != nil {
		repo.PruneCronExpr = *req.PruneCronExpr
	}
	if req.PruneArgs != nil {
		repo.PruneArgs = stringValue(*req.PruneArgs, defaultPruneArgs)
	}
	if req.CheckEnabled != nil {
		repo.CheckEnabled = *req.CheckEnabled
	}
	if req.CheckCronExpr != nil {
		repo.CheckCronExpr = *req.CheckCronExpr
	}
	if req.CheckArgs != nil {
		repo.CheckArgs = stringValue(*req.CheckArgs, defaultCheckArgs)
	}
	if err := s.ensureRepoUnique(repo.Name, repo.Type, repo.Endpoint, repo.ID); err != nil {
		return err
	}
	if err := s.store.Update(repo); err != nil {
		if isUniqueConstraintError(err) {
			return ErrRepositoryAlreadyExists
		}
		return err
	}
	if needsResync {
		if err := s.cache.ResetRepository(id); err != nil {
			return err
		}
		_, _ = s.cache.QueueInitialImport(id)
	}
	return nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unique") || strings.Contains(lower, "constraint")
}

func normalizeRepoEndpoint(req model.CreateRepoRequest) (string, error) {
	switch req.Type {
	case "local", "rclone":
		if strings.TrimSpace(req.Endpoint) == "" {
			return "", fmt.Errorf("endpoint required for %s repository", req.Type)
		}
		return strings.TrimSpace(req.Endpoint), nil
	case "webdav":
		if strings.TrimSpace(req.WebdavURL) == "" {
			return "", fmt.Errorf("webdav_url required for webdav repository")
		}
		if strings.TrimSpace(req.Endpoint) != "" {
			return strings.TrimSpace(req.Endpoint), nil
		}
		return strings.TrimSpace(req.WebdavURL), nil
	default:
		return "", fmt.Errorf("unsupported repository type: %s", req.Type)
	}
}

func normalizeProbeEndpoint(req model.RepositoryAccessRequest) (string, error) {
	switch req.Type {
	case "local", "rclone":
		if strings.TrimSpace(req.Endpoint) == "" {
			return "", fmt.Errorf("endpoint required for %s repository", req.Type)
		}
		return strings.TrimSpace(req.Endpoint), nil
	case "webdav":
		if strings.TrimSpace(req.WebdavURL) == "" {
			return "", fmt.Errorf("webdav_url required for webdav repository")
		}
		if strings.TrimSpace(req.Endpoint) != "" {
			return strings.TrimSpace(req.Endpoint), nil
		}
		return strings.TrimSpace(req.WebdavURL), nil
	default:
		return "", fmt.Errorf("unsupported repository type: %s", req.Type)
	}
}

func buildAccessProbeEnv(req model.RepositoryAccessRequest, endpoint string) ([]string, func(), error) {
	env := []string{"RESTIC_PASSWORD=" + req.Password}
	cleanup := func() {}

	switch req.Type {
	case "local":
		env = append(env, "RESTIC_REPOSITORY="+endpoint)
	case "rclone":
		env = append(env, "RESTIC_REPOSITORY=rclone:"+endpoint)
		if strings.TrimSpace(req.RcloneConfig) != "" {
			configPath, err := writeTempRcloneConfig(req.RcloneConfig)
			if err != nil {
				return nil, nil, err
			}
			env = append(env, "RCLONE_CONFIG="+configPath)
			cleanup = func() { _ = os.Remove(configPath) }
		}
	case "webdav":
		env = append(env, "RESTIC_REPOSITORY=webdav:"+req.WebdavURL)
		if req.WebdavUser != "" {
			env = append(env, "WEBDAV_USER="+req.WebdavUser)
		}
		if req.WebdavPassword != "" {
			env = append(env, "WEBDAV_PASSWORD="+req.WebdavPassword)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported repository type: %s", req.Type)
	}

	return env, cleanup, nil
}

func (s *RepoService) Delete(id int64) error {
	return s.store.Delete(id)
}

func (s *RepoService) buildResticRuntime(repo *model.Repository) ([]string, func(), error) {
	password, err := s.decrypt(repo.PasswordEncrypted)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt password: %w", err)
	}

	env := []string{
		"RESTIC_PASSWORD=" + password,
	}
	cleanup := func() {}

	switch repo.Type {
	case "local":
		env = append(env, "RESTIC_REPOSITORY="+repo.Endpoint)
	case "rclone":
		env = append(env, "RESTIC_REPOSITORY=rclone:"+repo.Endpoint)
		if strings.TrimSpace(repo.RcloneConfigEncrypted) != "" {
			rcloneConfig, err := s.decrypt(repo.RcloneConfigEncrypted)
			if err != nil {
				return nil, nil, fmt.Errorf("decrypt rclone config: %w", err)
			}
			configPath, err := writeTempRcloneConfig(rcloneConfig)
			if err != nil {
				return nil, nil, err
			}
			env = append(env, "RCLONE_CONFIG="+configPath)
			cleanup = func() {
				_ = os.Remove(configPath)
			}
		}
	case "webdav":
		env = append(env, "RESTIC_REPOSITORY=webdav:"+repo.WebdavURL)
		if repo.WebdavUser != "" {
			env = append(env, "WEBDAV_USER="+repo.WebdavUser)
		}
		if repo.WebdavPasswordEncrypted != "" {
			wdPwd, err := s.decrypt(repo.WebdavPasswordEncrypted)
			if err != nil {
				cleanup()
				return nil, nil, err
			}
			env = append(env, "WEBDAV_PASSWORD="+wdPwd)
		}
	}

	return env, cleanup, nil
}

// RunResticCommand executes a restic command against a specific repository.
func (s *RepoService) RunResticCommand(ctx context.Context, repoID int64, trigger string, args []string, cb executor.StreamCallback) (executor.ExecResult, error) {
	return s.runResticCommand(ctx, repoID, nil, trigger, args, cb, nil)
}

func (s *RepoService) RunTaskResticCommand(ctx context.Context, repoID int64, taskID int64, trigger string, args []string, cb executor.StreamCallback, started chan<- int64) (executor.ExecResult, error) {
	return s.runResticCommand(ctx, repoID, &taskID, trigger, args, cb, started)
}

func (s *RepoService) runResticCommand(ctx context.Context, repoID int64, taskID *int64, trigger string, args []string, cb executor.StreamCallback, started chan<- int64) (executor.ExecResult, error) {
	return s.runResticCommandWithReservation(ctx, repoID, taskID, trigger, args, cb, started, nil)
}

func (s *RepoService) runResticCommandWithReservation(ctx context.Context, repoID int64, taskID *int64, trigger string, args []string, cb executor.StreamCallback, started chan<- int64, reservation *repoOperationReservation) (executor.ExecResult, error) {
	if reservation == nil {
		var err error
		reservation, err = s.acquireRepoOperationForArgs(ctx, repoID, taskID, args, true)
		if err != nil {
			return executor.ExecResult{}, err
		}
	}
	defer s.releaseRepoOperation(reservation)

	repo, err := s.store.GetByID(repoID)
	if err != nil {
		return executor.ExecResult{}, fmt.Errorf("get repo: %w", err)
	}

	env, cleanup, err := s.buildResticRuntime(repo)
	if err != nil {
		return executor.ExecResult{}, fmt.Errorf("build env: %w", err)
	}
	defer cleanup()

	if trigger == "scheduled" && scheduledCommandNeedsLock(args) {
		if err := s.prepareScheduledResticLocks(ctx, repoID, args, env); err != nil {
			return executor.ExecResult{}, err
		}
	}

	result := s.executor.Run(ctx, executor.ExecRequest{
		RepoID:   &repoID,
		TaskID:   taskID,
		Trigger:  trigger,
		Args:     args,
		Env:      env,
		Callback: cb,
		Hub:      s.hub,
		Started:  started,
	})
	return result, nil
}

func scheduledCommandNeedsLock(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "backup", "check", "prune", "forget":
		return true
	default:
		return false
	}
}

func (s *RepoService) prepareScheduledResticLocks(ctx context.Context, repoID int64, args []string, env []string) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		locks, raw, err := s.listRepositoryLocks(ctx, env)
		if err != nil {
			return err
		}
		if len(locks) == 0 && !hasResticLockIDs(raw) {
			return nil
		}
		if s.hasCurrentInstanceLiveLock(locks) {
			command := strings.Join(args, " ")
			log.Printf("repo %d scheduled %s waiting for current AutoRestic lock to release", repoID, command)
			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		log.Printf("repo %d scheduled command found non-current or stale restic lock; running restic unlock --remove-all before execution", repoID)
		if err := s.unlockRepositoryLocks(ctx, repoID, env); err != nil {
			return err
		}
	}
}

func (s *RepoService) listRepositoryLocks(ctx context.Context, env []string) ([]resticLockInfo, string, error) {
	result := s.executor.Run(ctx, executor.ExecRequest{
		Trigger: "system_query",
		Args:    []string{"list", "locks", "--no-lock", "--json"},
		Env:     env,
		Hub:     s.hub,
	})
	output := strings.TrimSpace(result.Stdout)
	if output == "" {
		output = strings.TrimSpace(result.CombinedOutput)
	}
	if output == "" {
		output = strings.TrimSpace(result.Stderr)
	}
	if result.Err != nil {
		return nil, output, result.Err
	}
	if result.ExitCode != 0 {
		if isLockError(output) || result.ExitCode == 11 {
			return []resticLockInfo{{}}, output, nil
		}
		return nil, output, errors.New(strings.TrimSpace(output))
	}
	locks := parseResticLocks(output)
	if len(locks) == 0 && hasResticLockIDs(output) {
		locks = []resticLockInfo{{}}
	}
	return locks, output, nil
}

func parseResticLocks(output string) []resticLockInfo {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	var locks []resticLockInfo
	if err := json.Unmarshal([]byte(output), &locks); err == nil {
		return locks
	}
	var wrapped struct {
		Locks []resticLockInfo `json:"locks"`
	}
	if err := json.Unmarshal([]byte(output), &wrapped); err == nil && wrapped.Locks != nil {
		return wrapped.Locks
	}
	var single resticLockInfo
	if err := json.Unmarshal([]byte(output), &single); err == nil {
		return []resticLockInfo{single}
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item resticLockInfo
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			locks = append(locks, item)
		}
	}
	return locks
}

func (s *RepoService) hasCurrentInstanceLiveLock(locks []resticLockInfo) bool {
	hostname, err := os.Hostname()
	if err != nil {
		return false
	}
	for _, lock := range locks {
		if lock.Hostname != hostname || lock.PID <= 0 {
			continue
		}
		status := resticLockProcessStatus(lock.PID)
		if status.CurrentInstance && status.Alive {
			return true
		}
	}
	return false
}

func resticLockProcessStatus(pid int) resticLockOwnerStatus {
	if pid <= 0 {
		return resticLockOwnerStatus{}
	}
	if pid == os.Getpid() {
		return resticLockOwnerStatus{CurrentInstance: true, Alive: true}
	}
	if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err != nil {
		return resticLockOwnerStatus{}
	}
	currentPID := os.Getpid()
	for parentPID := pid; parentPID > 1; {
		if parentPID == currentPID {
			return resticLockOwnerStatus{CurrentInstance: true, Alive: true}
		}
		next, err := readProcParentPID(parentPID)
		if err != nil || next <= 0 || next == parentPID {
			return resticLockOwnerStatus{Alive: true}
		}
		parentPID = next
	}
	return resticLockOwnerStatus{Alive: true}
}

func readProcParentPID(pid int) (int, error) {
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	end := bytes.LastIndexByte(content, ')')
	if end < 0 || end+2 >= len(content) {
		return 0, fmt.Errorf("invalid proc stat for pid %d", pid)
	}
	fields := strings.Fields(string(content[end+2:]))
	if len(fields) < 2 {
		return 0, fmt.Errorf("invalid proc stat fields for pid %d", pid)
	}
	return strconv.Atoi(fields[1])
}

func (s *RepoService) unlockRepositoryLocks(ctx context.Context, repoID int64, env []string) error {
	result := s.executor.Run(ctx, executor.ExecRequest{
		RepoID:  &repoID,
		Trigger: "scheduled",
		Args:    []string{"unlock", "--remove-all"},
		Env:     env,
		Hub:     s.hub,
	})
	if result.Err != nil {
		return result.Err
	}
	if result.ExitCode != 0 {
		return errors.New(strings.TrimSpace(result.Stderr + "\n" + result.Stdout + "\n" + result.CombinedOutput))
	}
	return nil
}

func (s *RepoService) acquireRepoOperationForArgs(ctx context.Context, repoID int64, taskID *int64, args []string, wait bool) (*repoOperationReservation, error) {
	kind, class := classifyRepoOperation(taskID, args)
	return s.acquireRepoOperation(ctx, repoID, kind, class, wait)
}

func (s *RepoService) acquireRepoOperation(ctx context.Context, repoID int64, kind string, class repoOperationClass, wait bool) (*repoOperationReservation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		s.repoOpMu.Lock()
		if s.repoOps == nil {
			s.repoOps = map[int64]*repoOperationState{}
		}
		if active := s.repoOps[repoID]; active == nil {
			state := &repoOperationState{
				class: class,
				kind:  kind,
				done:  make(chan struct{}),
			}
			s.repoOps[repoID] = state
			s.repoOpMu.Unlock()
			return &repoOperationReservation{repoID: repoID, state: state}, nil
		} else {
			blockingKind := active.kind
			waitCh := active.done
			s.repoOpMu.Unlock()
			if !wait {
				return nil, fmt.Errorf("repository %d is busy with %s", repoID, blockingKind)
			}
			select {
			case <-waitCh:
			case <-ctx.Done():
				return nil, fmt.Errorf("wait for repository %d (%s) blocked by %s: %w", repoID, kind, blockingKind, ctx.Err())
			}
		}
	}
}

func (s *RepoService) releaseRepoOperation(reservation *repoOperationReservation) {
	if reservation == nil || reservation.state == nil {
		return
	}
	s.repoOpMu.Lock()
	defer s.repoOpMu.Unlock()
	active := s.repoOps[reservation.repoID]
	if active != reservation.state {
		return
	}
	delete(s.repoOps, reservation.repoID)
	close(reservation.state.done)
}

func classifyRepoOperation(taskID *int64, args []string) (string, repoOperationClass) {
	if taskID != nil {
		return "backup", repoOperationExclusive
	}
	if len(args) == 0 {
		return "unknown", repoOperationExclusive
	}
	switch args[0] {
	case "backup":
		return "backup", repoOperationExclusive
	case "forget":
		return "forget", repoOperationExclusive
	case "prune":
		return "prune", repoOperationExclusive
	case "check":
		return "check", repoOperationExclusive
	case "init":
		return "init", repoOperationExclusive
	case "unlock":
		return "unlock", repoOperationExclusive
	case "ls":
		return "sync:files", repoOperationRead
	case "stats":
		return "sync:stats", repoOperationRead
	case "snapshots":
		return "sync:core", repoOperationRead
	case "key":
		return "sync:keys", repoOperationRead
	case "cat":
		if len(args) > 1 && args[1] == "config" {
			return "sync:config", repoOperationRead
		}
		return "cat", repoOperationRead
	default:
		return args[0], repoOperationExclusive
	}
}

func (s *RepoService) InitRepo(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	result, err := s.RunResticCommand(ctx, repoID, "manual", []string{"init"}, nil)
	if err == nil && result.ExitCode == 0 {
		_, _ = s.cache.QueueInitialImport(repoID)
	}
	return result, err
}

func (s *RepoService) InitRepoAsync(repoID int64) (int64, error) {
	return s.runRepoCommandAsync(repoID, "manual", []string{"init"}, func(result executor.ExecResult, err error) {
		if err == nil && result.ExitCode == 0 {
			_, _ = s.cache.QueueInitialImport(repoID)
		}
	})
}

func (s *RepoService) CheckRepo(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	repo, err := s.store.GetByID(repoID)
	if err != nil {
		return executor.ExecResult{}, err
	}
	args, err := buildMaintenanceArgs("check", repo.CheckArgs)
	if err != nil {
		return executor.ExecResult{}, err
	}
	_ = s.cache.setSyncState(repoID, syncDomainCheck, syncStatusRunning, "", 10, 0, nil, "", nil)
	result, runErr := s.RunResticCommand(ctx, repoID, "manual", args, nil)
	s.cache.RecordCheckResult(repoID, runErr, result.ExitCode, result.LogID)
	return result, runErr
}

func (s *RepoService) CheckRepoAsync(repoID int64) (int64, error) {
	repo, err := s.store.GetByID(repoID)
	if err != nil {
		return 0, err
	}
	args, err := buildMaintenanceArgs("check", repo.CheckArgs)
	if err != nil {
		return 0, err
	}
	_ = s.cache.setSyncState(repoID, syncDomainCheck, syncStatusRunning, "", 10, 0, nil, "", nil)
	logID, err := s.runRepoCommandAsync(repoID, "manual", args, func(result executor.ExecResult, err error) {
		s.cache.RecordCheckResult(repoID, err, result.ExitCode, result.LogID)
	})
	if err != nil {
		_ = s.cache.setSyncState(repoID, syncDomainCheck, syncStatusFailed, "", 100, 0, nil, err.Error(), nil)
		return 0, err
	}
	_ = s.cache.setSyncState(repoID, syncDomainCheck, syncStatusRunning, "", 10, 0, nil, "", &logID)
	return logID, nil
}

func (s *RepoService) UnlockRepo(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	return s.RunResticCommand(ctx, repoID, "manual", []string{"unlock"}, nil)
}

func (s *RepoService) UnlockRepoAsync(repoID int64) (int64, error) {
	return s.runRepoCommandAsync(repoID, "manual", []string{"unlock"}, nil)
}

func (s *RepoService) GetStats(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	return s.GetStatsCached(ctx, repoID, false)
}

func (s *RepoService) GetStatsRaw(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	return s.RunResticCommand(ctx, repoID, "system_query", []string{"stats", "--json"}, nil)
}

func (s *RepoService) GetStatsCached(ctx context.Context, repoID int64, refresh bool) (executor.ExecResult, error) {
	view, err := s.cache.GetStatsView(repoID, refresh)
	if err != nil {
		return executor.ExecResult{ExitCode: 1, Err: err}, err
	}
	exitCode := 0
	var resultErr error
	if view.Stale && strings.TrimSpace(view.Error) != "" {
		exitCode = 1
		resultErr = errors.New(view.Error)
	}
	return executor.ExecResult{ExitCode: exitCode, Stdout: string(view.Data), Stderr: view.Error, Err: resultErr}, resultErr
}

func (s *RepoService) PruneRepo(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	repo, err := s.store.GetByID(repoID)
	if err != nil {
		return executor.ExecResult{}, err
	}
	args, err := buildMaintenanceArgs("prune", repo.PruneArgs)
	if err != nil {
		return executor.ExecResult{}, err
	}
	result, err := s.RunResticCommand(ctx, repoID, "manual", args, nil)
	if err == nil && result.ExitCode == 0 {
		s.cache.MarkStale(repoID, syncDomainStats)
	}
	return result, err
}

func (s *RepoService) PruneRepoAsync(repoID int64) (int64, error) {
	repo, err := s.store.GetByID(repoID)
	if err != nil {
		return 0, err
	}
	args, err := buildMaintenanceArgs("prune", repo.PruneArgs)
	if err != nil {
		return 0, err
	}
	return s.runRepoCommandAsync(repoID, "manual", args, func(result executor.ExecResult, err error) {
		if err == nil && result.ExitCode == 0 {
			s.cache.MarkStale(repoID, syncDomainStats)
		}
	})
}

func (s *RepoService) ScheduledCheckRepo(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	repo, err := s.store.GetByID(repoID)
	if err != nil {
		return executor.ExecResult{}, err
	}
	args, err := buildMaintenanceArgs("check", repo.CheckArgs)
	if err != nil {
		return executor.ExecResult{}, err
	}
	_ = s.cache.setSyncState(repoID, syncDomainCheck, syncStatusRunning, "", 10, 0, nil, "", nil)
	result, runErr := s.RunResticCommand(ctx, repoID, "scheduled", args, nil)
	s.cache.RecordCheckResult(repoID, runErr, result.ExitCode, result.LogID)
	return result, runErr
}

func (s *RepoService) ScheduledPruneRepo(ctx context.Context, repoID int64) (executor.ExecResult, error) {
	repo, err := s.store.GetByID(repoID)
	if err != nil {
		return executor.ExecResult{}, err
	}
	args, err := buildMaintenanceArgs("prune", repo.PruneArgs)
	if err != nil {
		return executor.ExecResult{}, err
	}
	result, err := s.RunResticCommand(ctx, repoID, "scheduled", args, nil)
	if err == nil && result.ExitCode == 0 {
		s.cache.MarkStale(repoID, syncDomainStats)
	}
	return result, err
}

func buildMaintenanceArgs(command, raw string) ([]string, error) {
	args := []string{command}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return args, nil
	}
	var extra []string
	if err := json.Unmarshal([]byte(raw), &extra); err != nil {
		return nil, fmt.Errorf("parse %s args: %w", command, err)
	}
	return append(args, extra...), nil
}

func (s *RepoService) GetKeysCached(ctx context.Context, repoID int64, refresh bool) (RepoKeysView, error) {
	return s.cache.GetKeysView(repoID, refresh)
}

// CheckRepoPath checks if a path is already a restic repository
func (s *RepoService) CheckRepoPath(ctx context.Context, req model.RepositoryAccessRequest) (RepositoryProbe, error) {
	return s.ProbeRepositoryAccess(ctx, req)
}

func (s *RepoService) runRepoCommandAsync(repoID int64, trigger string, args []string, onDone func(executor.ExecResult, error)) (int64, error) {
	reservation, err := s.acquireRepoOperationForArgs(context.Background(), repoID, nil, args, false)
	if err != nil {
		return 0, err
	}
	started := make(chan int64, 1)
	startErr := make(chan error, 1)
	go func() {
		result, err := s.runResticCommandWithReservation(context.Background(), repoID, nil, trigger, args, nil, started, reservation)
		if onDone != nil {
			onDone(result, err)
		}
		if err != nil {
			startErr <- err
		}
	}()

	select {
	case logID := <-started:
		return logID, nil
	case err := <-startErr:
		return 0, err
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("repository command did not start in time")
	}
}

func writeTempRcloneConfig(contents string) (string, error) {
	file, err := os.CreateTemp("", "autorestic-rclone-*.conf")
	if err != nil {
		return "", fmt.Errorf("create rclone config: %w", err)
	}
	path := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(path)
	}
	if err := file.Chmod(0600); err != nil {
		cleanup()
		return "", fmt.Errorf("chmod rclone config: %w", err)
	}
	if _, err := file.WriteString(contents); err != nil {
		cleanup()
		return "", fmt.Errorf("write rclone config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close rclone config: %w", err)
	}
	return path, nil
}
