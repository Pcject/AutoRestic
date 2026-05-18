package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/repository"
	"github.com/robfig/cron/v3"
)

type TaskService struct {
	store       *repository.TaskStore
	repoSvc     *RepoService
	activeMu    sync.Mutex
	activeTasks map[int64]bool
}

const defaultForgetPolicy = `{"keep-last":"unlimited"}`

func NewTaskService(store *repository.TaskStore, repoSvc *RepoService) *TaskService {
	return &TaskService{
		store:       store,
		repoSvc:     repoSvc,
		activeTasks: map[int64]bool{},
	}
}

func (s *TaskService) List(q model.TaskQuery) ([]model.BackupTask, error) {
	return s.store.List(q)
}

func (s *TaskService) GetByID(id int64) (*model.BackupTask, error) {
	return s.store.GetByID(id)
}

func (s *TaskService) Create(req model.CreateTaskRequest) (int64, error) {
	nextRun := s.calculateNextRun(req.CronExpr, req.CronEnabled)
	task := &model.BackupTask{
		RepoID:       req.RepoID,
		Name:         req.Name,
		SourcePaths:  req.SourcePaths,
		Excludes:     req.Excludes,
		Tags:         req.Tags,
		CronExpr:     req.CronExpr,
		CronEnabled:  req.CronEnabled,
		ForgetPolicy: req.ForgetPolicy,
		PreHooks:     req.PreHooks,
		PostHooks:    req.PostHooks,
		ExtraFlags:   req.ExtraFlags,
		NextRunAt:    nextRun,
	}
	if task.ForgetPolicy == "" {
		task.ForgetPolicy = defaultForgetPolicy
	}
	if task.PreHooks == "" {
		task.PreHooks = "[]"
	}
	if task.PostHooks == "" {
		task.PostHooks = "[]"
	}
	if task.ExtraFlags == "" {
		task.ExtraFlags = "{}"
	}
	return s.store.Create(task)
}

func (s *TaskService) Update(id int64, req model.UpdateTaskRequest) error {
	task, err := s.store.GetByID(id)
	if err != nil {
		return err
	}
	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.SourcePaths != nil {
		task.SourcePaths = *req.SourcePaths
	}
	if req.Excludes != nil {
		task.Excludes = *req.Excludes
	}
	if req.Tags != nil {
		task.Tags = *req.Tags
	}
	if req.CronExpr != nil {
		task.CronExpr = *req.CronExpr
	}
	if req.CronEnabled != nil {
		task.CronEnabled = *req.CronEnabled
	}
	if req.ForgetPolicy != nil {
		task.ForgetPolicy = *req.ForgetPolicy
	}
	if req.PreHooks != nil {
		task.PreHooks = *req.PreHooks
	}
	if req.PostHooks != nil {
		task.PostHooks = *req.PostHooks
	}
	if req.ExtraFlags != nil {
		task.ExtraFlags = *req.ExtraFlags
	}
	task.NextRunAt = s.calculateNextRun(task.CronExpr, task.CronEnabled)
	return s.store.Update(task)
}

func (s *TaskService) Delete(id int64) error {
	return s.store.Delete(id)
}

func (s *TaskService) Toggle(ctx context.Context, id int64) error {
	task, err := s.store.GetByID(id)
	if err != nil {
		return err
	}
	task.CronEnabled = !task.CronEnabled
	task.NextRunAt = s.calculateNextRun(task.CronExpr, task.CronEnabled)
	return s.store.Update(task)
}

func (s *TaskService) RunTask(ctx context.Context, taskID int64, trigger string) (executor.ExecResult, error) {
	task, err := s.store.GetByID(taskID)
	if err != nil {
		return executor.ExecResult{}, err
	}

	args, err := buildBackupArgs(task)
	if err != nil {
		return executor.ExecResult{}, err
	}
	if !s.markTaskActive(taskID) {
		return executor.ExecResult{}, fmt.Errorf("task %d is already running", taskID)
	}
	defer s.clearTaskActive(taskID)

	if trigger == "" {
		trigger = "manual"
	}
	if hookResult, err := s.runHooks(ctx, task, trigger, "pre", task.PreHooks); err != nil {
		return hookResult, err
	}
	result, err := s.repoSvc.RunTaskResticCommand(ctx, task.RepoID, task.ID, trigger, args, nil, nil)
	if err != nil {
		return executor.ExecResult{}, err
	}

	s.finishTaskRun(ctx, task, trigger, result)
	if hookResult, hookErr := s.runHooks(ctx, task, trigger, "post", task.PostHooks); hookErr != nil {
		return mergeHookFailure(result, hookResult, hookErr), hookErr
	}

	return result, nil
}

func (s *TaskService) RunTaskAsync(taskID int64, trigger string) (int64, error) {
	task, err := s.store.GetByID(taskID)
	if err != nil {
		return 0, err
	}
	args, err := buildBackupArgs(task)
	if err != nil {
		return 0, err
	}
	if trigger == "" {
		trigger = "manual"
	}
	if !s.markTaskActive(taskID) {
		return 0, fmt.Errorf("task %d is already running", taskID)
	}

	started := make(chan int64, 1)
	startErr := make(chan error, 1)
	go func() {
		defer s.clearTaskActive(taskID)
		bg := context.Background()
		if _, err := s.runHooks(bg, task, trigger, "pre", task.PreHooks); err != nil {
			startErr <- err
			return
		}
		reservation, err := s.repoSvc.acquireRepoOperationForArgs(bg, task.RepoID, &task.ID, args, false)
		if err != nil {
			startErr <- err
			return
		}
		result, err := s.repoSvc.runResticCommandWithReservation(bg, task.RepoID, &task.ID, trigger, args, nil, started, reservation)
		if err != nil {
			startErr <- err
			return
		}
		if err == nil {
			s.finishTaskRun(context.Background(), task, trigger, result)
			_, _ = s.runHooks(context.Background(), task, trigger, "post", task.PostHooks)
		}
	}()

	select {
	case logID := <-started:
		return logID, nil
	case err := <-startErr:
		return 0, err
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("task did not start in time")
	}
}

func buildBackupArgs(task *model.BackupTask) ([]string, error) {
	var sourcePaths []string
	if err := json.Unmarshal([]byte(task.SourcePaths), &sourcePaths); err != nil {
		return nil, fmt.Errorf("parse source paths: %w", err)
	}
	if len(sourcePaths) == 0 {
		return nil, fmt.Errorf("source paths required")
	}
	var excludes []string
	if err := json.Unmarshal([]byte(task.Excludes), &excludes); err != nil {
		return nil, fmt.Errorf("parse excludes: %w", err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(task.Tags), &tags); err != nil {
		return nil, fmt.Errorf("parse tags: %w", err)
	}

	args := []string{"backup"}
	extraArgs, err := buildExtraFlagArgs(task.ExtraFlags)
	if err != nil {
		return nil, err
	}
	args = append(args, extraArgs...)
	if !containsArg(args, "--json") {
		args = append(args, "--json")
	}
	for _, ex := range excludes {
		args = append(args, "--exclude", ex)
	}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}
	args = append(args, sourcePaths...)
	return args, nil
}

func (s *TaskService) finishTaskRun(ctx context.Context, task *model.BackupTask, trigger string, result executor.ExecResult) {
	// Update last run time
	now := time.Now()
	nextRun := s.calculateNextRun(task.CronExpr, task.CronEnabled)
	s.store.UpdateRunTimes(task.ID, now, nextRun)

	if result.ExitCode == 0 {
		_ = s.repoSvc.Cache().StoreBackupSummary(task.RepoID, result.Stdout)
	}
	// Run forget policy if configured
	if task.ForgetPolicy != "{}" && task.ForgetPolicy != "" && result.ExitCode == 0 {
		s.runForgetPolicy(ctx, task, trigger)
	}
	if result.ExitCode == 0 {
		s.repoSvc.Cache().MarkStale(task.RepoID, syncDomainCore, syncDomainStats, syncDomainFiles)
		_, _ = s.repoSvc.Cache().QueueCoreSync(task.RepoID)
	}
}

func (s *TaskService) markTaskActive(taskID int64) bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if s.activeTasks[taskID] {
		return false
	}
	s.activeTasks[taskID] = true
	return true
}

func (s *TaskService) clearTaskActive(taskID int64) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	delete(s.activeTasks, taskID)
}

func (s *TaskService) runForgetPolicy(ctx context.Context, task *model.BackupTask, trigger string) {
	var policy map[string]any
	if err := json.Unmarshal([]byte(task.ForgetPolicy), &policy); err != nil {
		return
	}

	args := []string{"forget"}
	if v, ok := policyValue(policy, "keep_last", "keep-last"); ok {
		args = append(args, "--keep-last", v)
	}
	if v, ok := policyValue(policy, "keep_daily", "keep-daily"); ok {
		args = append(args, "--keep-daily", v)
	}
	if v, ok := policyValue(policy, "keep_weekly", "keep-weekly"); ok {
		args = append(args, "--keep-weekly", v)
	}
	if v, ok := policyValue(policy, "keep_monthly", "keep-monthly"); ok {
		args = append(args, "--keep-monthly", v)
	}
	if v, ok := policyValue(policy, "keep_yearly", "keep-yearly"); ok {
		args = append(args, "--keep-yearly", v)
	}
	if len(args) == 1 {
		return
	}
	result, err := s.repoSvc.RunResticCommand(ctx, task.RepoID, trigger, args, nil)
	if err == nil && result.ExitCode == 0 {
		s.repoSvc.Cache().MarkStale(task.RepoID, syncDomainCore, syncDomainStats, syncDomainFiles)
		_, _ = s.repoSvc.Cache().QueueCoreSync(task.RepoID)
	}
}

func policyValue(policy map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := policy[key]; ok {
			switch v := value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return v, true
				}
			case float64:
				if v > 0 {
					return strconv.FormatFloat(v, 'f', -1, 64), true
				}
			}
		}
	}
	return "", false
}

var allowedExtraFlags = map[string]bool{
	"--host":                true,
	"--compression":         true,
	"--pack-size":           true,
	"--parent":              true,
	"--force":               true,
	"--limit-upload":        true,
	"--limit-download":      true,
	"--read-concurrency":    true,
	"--ignore-ctime":        true,
	"--ignore-inode":        true,
	"--one-file-system":     true,
	"--no-scan":             true,
	"--with-atime":          true,
	"--exclude-caches":      true,
	"--exclude-file":        true,
	"--files-from":          true,
	"--exclude-larger-than": true,
	"--exclude-if-present":  true,
	"--iexclude":            true,
	"--iexclude-file":       true,
	"--verbose":             true,
	"--quiet":               true,
	"--json":                true,
	"--no-lock":             true,
	"--retry-lock":          true,
	"--dry-run":             true,
	"--group-by":            true,
	"--skip-if-unchanged":   true,
	"--no-cache":            true,
	"--option":              true,
}

func buildExtraFlagArgs(raw string) ([]string, error) {
	if raw == "" || raw == "{}" {
		return nil, nil
	}

	var flags map[string]any
	if err := json.Unmarshal([]byte(raw), &flags); err != nil {
		return nil, fmt.Errorf("parse extra flags: %w", err)
	}

	keys := make([]string, 0, len(flags))
	for key := range flags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	args := []string{}
	for _, key := range keys {
		if !allowedExtraFlags[key] {
			return nil, fmt.Errorf("unsupported restic flag: %s", key)
		}
		value := flags[key]
		switch v := value.(type) {
		case bool:
			if v {
				args = append(args, key)
			}
		case string:
			if v != "" {
				args = append(args, key, v)
			}
		case float64:
			if key == "--verbose" && v == 2 {
				args = append(args, "-vv")
			} else {
				args = append(args, key, strconv.FormatFloat(v, 'f', -1, 64))
			}
		default:
			return nil, fmt.Errorf("unsupported value for restic flag %s", key)
		}
	}
	return args, nil
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func (s *TaskService) calculateNextRun(cronExpr string, enabled bool) *time.Time {
	if !enabled || cronExpr == "" {
		return nil
	}
	// Parse cron expression and calculate next run
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil
	}
	next := schedule.Next(time.Now())
	return &next
}

func (s *TaskService) runHooks(ctx context.Context, task *model.BackupTask, trigger, phase, raw string) (executor.ExecResult, error) {
	hooks, err := parseHookCommands(raw)
	if err != nil {
		return executor.ExecResult{}, fmt.Errorf("parse %s hooks: %w", phase, err)
	}
	if len(hooks) == 0 {
		return executor.ExecResult{}, nil
	}

	repoID := task.RepoID
	taskID := task.ID
	var last executor.ExecResult
	for i, hook := range hooks {
		hookLabel := fmt.Sprintf("%s-hook #%d", phase, i+1)
		last = s.repoSvc.executor.Run(ctx, executor.ExecRequest{
			RepoID:  &repoID,
			TaskID:  &taskID,
			Trigger: trigger,
			Binary:  shellBinary(),
			Args:    []string{"-c", hook},
			Command: hookLabel,
			Env: []string{
				"AUTORESTIC_HOOK_PHASE=" + phase,
				"AUTORESTIC_TASK_ID=" + strconv.FormatInt(taskID, 10),
				"AUTORESTIC_REPO_ID=" + strconv.FormatInt(repoID, 10),
				"AUTORESTIC_TRIGGER=" + trigger,
			},
			Hub: s.repoSvc.hub,
		})
		if err := hookExecError(hookLabel, last); err != nil {
			return last, err
		}
	}
	return last, nil
}

func parseHookCommands(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var hooks []string
	if err := json.Unmarshal([]byte(raw), &hooks); err != nil {
		return nil, err
	}
	commands := make([]string, 0, len(hooks))
	for _, hook := range hooks {
		trimmed := strings.TrimSpace(hook)
		if trimmed != "" {
			commands = append(commands, trimmed)
		}
	}
	return commands, nil
}

func hookExecError(hookLabel string, result executor.ExecResult) error {
	if result.Err != nil {
		return fmt.Errorf("%s failed: %w", hookLabel, result.Err)
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if message != "" {
			return fmt.Errorf("%s failed: %s", hookLabel, message)
		}
		return fmt.Errorf("%s failed with exit code %d", hookLabel, result.ExitCode)
	}
	return nil
}

func mergeHookFailure(result executor.ExecResult, hookResult executor.ExecResult, hookErr error) executor.ExecResult {
	if hookErr == nil {
		return result
	}
	merged := result
	if merged.ExitCode == 0 {
		merged.ExitCode = hookResult.ExitCode
		if merged.ExitCode == 0 {
			merged.ExitCode = 1
		}
	}
	if merged.Err == nil {
		merged.Err = hookErr
	} else {
		merged.Err = errors.Join(merged.Err, hookErr)
	}
	if hookResult.Stderr != "" {
		if merged.Stderr != "" && !strings.HasSuffix(merged.Stderr, "\n") {
			merged.Stderr += "\n"
		}
		merged.Stderr += hookResult.Stderr
	}
	if hookResult.Stdout != "" {
		if merged.CombinedOutput != "" && !strings.HasSuffix(merged.CombinedOutput, "\n") {
			merged.CombinedOutput += "\n"
		}
		merged.CombinedOutput += hookResult.Stdout
	}
	return merged
}

func shellBinary() string {
	if _, err := os.Stat("/bin/sh"); err == nil {
		return "/bin/sh"
	}
	return "sh"
}
