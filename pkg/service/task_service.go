package service

import (
	"fmt"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
)

type TaskService struct {
	store  storage.Store
	logger Logger
}

func NewTaskService(store storage.Store, logger Logger) *TaskService {
	return &TaskService{
		store:  store,
		logger: logger,
	}
}

func (ts *TaskService) CanRunTask(task models.Task) (bool, error) {
	for _, dep := range task.Dependencies {
		d, err := ts.store.GetTask(dep, task.WorkflowID)
		if err != nil {
			ts.logger.Errorf("Error retrieving dependency %s: %v", dep, err)
			return false, fmt.Errorf("failed to retrie dependency %s: %v", dep, err)
		}
		if d.Status != "COMPLETED" {
			ts.logger.Infof("Cannot run task %s as dependency %s is not in status COMPLETED", task.Name, dep)
			return false, nil
		}
	}
	return true, nil
}

func (ts *TaskService) SaveTask(task models.Task) (err error) {
	txStore, err := ts.store.Begin()
	if err != nil {
		ts.logger.Errorf("Failed to begin transaction for SaveTask: %v", err)
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := txStore.Rollback(); rollbackErr != nil {
				ts.logger.Errorf("Failed to rollback: %v", err)
			}
		} else {
			if commitErr := txStore.Commit(); commitErr != nil {
				ts.logger.Errorf("Failed to commit: %v", err)
				err = commitErr
			}
		}
	}()

	if err = txStore.SaveTask(task); err != nil {
		ts.logger.Errorf("Failed to save task %s: %v", task.ID, err)
		return fmt.Errorf("failed to save task %s: %v", task.ID, err)
	}
	return nil
}

func (ts *TaskService) UpdateTaskStatus(taskID string, workflowID int64, status, errMsg string) (err error) {
	txStore, err := ts.store.Begin()
	if err != nil {
		ts.logger.Errorf("Failed to begin transaction for UpdateTaskStatus: %v", err)
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := txStore.Rollback(); rollbackErr != nil {
				ts.logger.Errorf("Failed to rollback: %v", rollbackErr)
			}
		} else {
			if commitErr := txStore.Commit(); commitErr != nil {
				ts.logger.Errorf("Failed to commit: %v", commitErr)
				err = commitErr
			}
		}
	}()

	if err = txStore.UpdateTaskStatus(taskID, workflowID, status, errMsg); err != nil {
		ts.logger.Errorf("Failed to update task %s status to %s: %v", taskID, status, err)
		return fmt.Errorf("failed to update task %s status: %v", taskID, err)
	}
	return nil
}
