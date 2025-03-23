package service

import (
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
)

type WorkflowService struct {
	store storage.Store
}

func NewWorkflowService(store storage.Store) *WorkflowService {
	return &WorkflowService{store: store}
}

func (s *WorkflowService) CreateWorkflow(name string) (id int64, err error) {
	txStore, err := s.store.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			if rollbackErr := txStore.Rollback(); rollbackErr != nil {
				log.GetLogger().Errorf("Failed to rollback after error: %v (original error: %v)", rollbackErr, err)
			}
			return
		}
		if commitErr := txStore.Commit(); commitErr != nil {
			log.GetLogger().Errorf("Failed to commit: %v", commitErr)
			err = commitErr // Update the named return value
		}
	}()

	wf := models.Workflow{
		Name:      name,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	id, err = txStore.SaveWorkflow(wf)
	if err != nil {
		return 0, err
	}
	log.GetLogger().Infof("Created workflow '%s' with ID %d", name, id)
	return id, nil
}

func (s *WorkflowService) ListWorkflows() ([]models.Workflow, error) {
	return s.store.ListWorkflows()
}
