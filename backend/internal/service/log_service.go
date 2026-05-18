// backend/internal/service/log_service.go
package service

import (
	"fmt"

	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/repository"
)

type LogService struct {
	store *repository.LogStore
	exec  *executor.Executor
}

func NewLogService(store *repository.LogStore, exec *executor.Executor) *LogService {
	return &LogService{store: store, exec: exec}
}

func (s *LogService) Query(q model.LogQuery) (*model.PaginatedResult, error) {
	return s.store.Query(q)
}

func (s *LogService) GetByID(id int64, outputLimit int) (*model.ExecutionLog, error) {
	return s.store.GetByID(id, outputLimit)
}

func (s *LogService) Cleanup(retainDays int) (int64, error) {
	return s.store.DeleteOlderThan(retainDays)
}

func (s *LogService) Cancel(id int64) error {
	status, err := s.store.GetStatusByID(id)
	if err != nil {
		return err
	}
	if status != "running" {
		return fmt.Errorf("log %d is not running", id)
	}
	if !s.exec.Cancel(id) {
		return fmt.Errorf("running command %d not found", id)
	}
	return nil
}
