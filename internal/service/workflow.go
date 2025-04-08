package service

import (
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/pkg/errors"
)

type WorkflowService struct {
	store storage.Store
}

func NewWorkflowService(store storage.Store) *WorkflowService {
	return &WorkflowService{store: store}
}

func (s *WorkflowService) CreateWorkflow(name string) (id int64, err error) {
	if name == "" {
		return 0, errors.New("workflow name cannot be empty")
	}
	if len(name) > 100 {
		return 0, errors.New("workflow name too long (max 100 characters)")
	}
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
		Status:    models.PendingWorkflowStatus,
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

// UpdateWorkflowStatus updates the status of an existing workflow by ID.
func (s *WorkflowService) UpdateWorkflowStatus(id int64, status string) error {
	// Validate inputs
	if id <= 0 {
		return errors.New("workflow ID must be positive")
	}
	wfStatus := models.WorkflowStatus(status)
	switch wfStatus {
	case models.PendingWorkflowStatus, models.RunningWorkflowStatus,
		models.CompletedWorkflowStatus, models.FailedWorkflowStatus:
		// Valid status, proceed
	default:
		return errors.New("invalid status; must be 'PENDING', 'RUNNING', 'COMPLETED', or 'FAILED'")
	}

	// Start transaction
	txStore, err := s.store.Begin()
	if err != nil {
		return err
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
			err = commitErr
		}
	}()

	// Fetch existing workflow to ensure it exists
	wf, err := txStore.GetWorkflow(id)
	if err != nil {
		return err
	}

	// Update status and timestamp
	if err := txStore.UpdateWorkflowStatus(wf.ID, wfStatus); err != nil {
		return err
	}

	log.GetLogger().Infof("Updated workflow ID %d to status '%s'", id, status)
	return nil
}

func (s *WorkflowService) ListWorkflows() ([]models.Workflow, error) {
	return s.store.ListWorkflows()
}
