package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/service"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	taskSvc  *service.TaskService
	repoSvc  *service.RepoService
	cron     *cron.Cron
	mu       sync.Mutex
	reloadMu sync.Mutex
	running  bool
	stopCh   chan struct{}

	listTasks         func() ([]model.BackupTask, error)
	listRepos         func() ([]model.Repository, error)
	runTask           func(context.Context, int64, string) (executor.ExecResult, error)
	runScheduledPrune func(context.Context, int64) (executor.ExecResult, error)
	runScheduledCheck func(context.Context, int64) (executor.ExecResult, error)

	maintenanceMu     sync.Mutex
	activeMaintenance map[int64]string
}

func NewScheduler(taskSvc *service.TaskService, repoSvc *service.RepoService) *Scheduler {
	return &Scheduler{
		taskSvc: taskSvc,
		repoSvc: repoSvc,
		cron:    cron.New(),
		stopCh:  make(chan struct{}),
		listTasks: func() ([]model.BackupTask, error) {
			return taskSvc.List(model.TaskQuery{CronEnabled: ptr(true)})
		},
		listRepos: repoSvc.List,
		runTask: func(ctx context.Context, taskID int64, trigger string) (executor.ExecResult, error) {
			return taskSvc.RunTask(ctx, taskID, trigger)
		},
		runScheduledPrune: repoSvc.ScheduledPruneRepo,
		runScheduledCheck: repoSvc.ScheduledCheckRepo,
		activeMaintenance: make(map[int64]string),
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	tasks, err := s.listTasks()
	if err != nil {
		log.Printf("scheduler: failed to load tasks: %v", err)
		return
	}

	repos, err := s.listRepos()
	if err != nil {
		log.Printf("scheduler: failed to load repository maintenance schedules: %v", err)
		repos = nil
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.cron = cron.New()
	for _, task := range tasks {
		s.scheduleTask(task)
	}
	for _, repo := range repos {
		s.scheduleRepoMaintenance(repo)
	}

	s.cron.Start()
	s.mu.Unlock()
	log.Printf("scheduler: started with %d tasks", len(tasks))

	// Background polling to pick up new/updated tasks every minute
	go s.pollLoop()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.cron.Stop()
	close(s.stopCh)
	s.running = false
	log.Println("scheduler: stopped")
}

func (s *Scheduler) scheduleTask(task model.BackupTask) {
	if task.CronExpr == "" {
		return
	}

	_, err := s.cron.AddFunc(task.CronExpr, func() {
		log.Printf("scheduler: triggering task %d (%s)", task.ID, task.Name)
		ctx := context.Background()
		result, err := s.runTask(ctx, task.ID, "scheduled")
		if err != nil {
			log.Printf("scheduler: task %d failed: %v", task.ID, err)
		} else if result.ExitCode != 0 {
			log.Printf("scheduler: task %d exited with code %d", task.ID, result.ExitCode)
		} else {
			log.Printf("scheduler: task %d completed successfully", task.ID)
		}
	})

	if err != nil {
		log.Printf("scheduler: failed to schedule task %d: %v", task.ID, err)
	}
}

func (s *Scheduler) scheduleRepoMaintenance(repo model.Repository) {
	if repo.PruneEnabled && repo.PruneCronExpr != "" {
		repoID := repo.ID
		repoName := repo.Name
		_, err := s.cron.AddFunc(repo.PruneCronExpr, func() {
			s.runRepoMaintenance(repoID, repoName, "prune", s.runScheduledPrune)
		})
		if err != nil {
			log.Printf("scheduler: failed to schedule prune for repo %d: %v", repo.ID, err)
		}
	}
	if repo.CheckEnabled && repo.CheckCronExpr != "" {
		repoID := repo.ID
		repoName := repo.Name
		_, err := s.cron.AddFunc(repo.CheckCronExpr, func() {
			s.runRepoMaintenance(repoID, repoName, "check", s.runScheduledCheck)
		})
		if err != nil {
			log.Printf("scheduler: failed to schedule check for repo %d: %v", repo.ID, err)
		}
	}
}

func logMaintenanceResult(kind string, repoID int64, result executor.ExecResult, err error) {
	if err != nil {
		log.Printf("scheduler: repo %d %s failed: %v", repoID, kind, err)
		return
	}
	if result.ExitCode != 0 {
		log.Printf("scheduler: repo %d %s exited with code %d", repoID, kind, result.ExitCode)
		return
	}
	log.Printf("scheduler: repo %d %s completed successfully", repoID, kind)
}

func (s *Scheduler) runRepoMaintenance(repoID int64, repoName, kind string, run func(context.Context, int64) (executor.ExecResult, error)) {
	if !s.startMaintenance(repoID, kind) {
		log.Printf("scheduler: skipping %s for repo %d (%s); %s already running", kind, repoID, repoName, s.currentMaintenance(repoID))
		return
	}
	defer s.finishMaintenance(repoID)

	log.Printf("scheduler: triggering %s for repo %d (%s)", kind, repoID, repoName)
	result, err := run(context.Background(), repoID)
	logMaintenanceResult(kind, repoID, result, err)
}

func (s *Scheduler) pollLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.reloadTasks()
		}
	}
}

func (s *Scheduler) reloadTasks() {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	tasks, err := s.listTasks()
	if err != nil {
		log.Printf("scheduler: failed to reload tasks: %v", err)
		return
	}

	repos, err := s.listRepos()
	if err != nil {
		log.Printf("scheduler: failed to reload repository maintenance schedules: %v", err)
		repos = nil
	}

	nextCron := cron.New()
	prevCron := s.swapCron(nextCron)
	for _, task := range tasks {
		s.scheduleTask(task)
	}
	for _, repo := range repos {
		s.scheduleRepoMaintenance(repo)
	}
	nextCron.Start()
	if prevCron != nil {
		prevCron.Stop()
	}
	log.Printf("scheduler: reloaded %d tasks", len(tasks))
}

func (s *Scheduler) swapCron(next *cron.Cron) *cron.Cron {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.cron
	s.cron = next
	return prev
}

func (s *Scheduler) startMaintenance(repoID int64, kind string) bool {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if _, exists := s.activeMaintenance[repoID]; exists {
		return false
	}
	s.activeMaintenance[repoID] = kind
	return true
}

func (s *Scheduler) currentMaintenance(repoID int64) string {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	return s.activeMaintenance[repoID]
}

func (s *Scheduler) finishMaintenance(repoID int64) {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	delete(s.activeMaintenance, repoID)
}

func ptr[T any](v T) *T {
	return &v
}
