package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/robfig/cron/v3"
)

func TestStartLoadFailureDoesNotLeaveSchedulerRunning(t *testing.T) {
	s := &Scheduler{
		cron:              cron.New(),
		stopCh:            make(chan struct{}),
		listTasks:         func() ([]model.BackupTask, error) { return nil, errors.New("boom") },
		activeMaintenance: map[int64]string{},
	}

	s.Start()

	if s.running {
		t.Fatal("expected scheduler to remain stopped after start failure")
	}
}

func TestRunRepoMaintenanceSkipsOverlappingOperationsPerRepo(t *testing.T) {
	s := &Scheduler{activeMaintenance: map[int64]string{}}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls int32

	run := func(context.Context, int64) (executor.ExecResult, error) {
		atomic.AddInt32(&calls, 1)
		close(started)
		<-release
		return executor.ExecResult{}, nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.runRepoMaintenance(42, "repo", "prune", run)
	}()

	<-started
	s.runRepoMaintenance(42, "repo", "check", run)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected only one maintenance invocation, got %d", got)
	}
}
