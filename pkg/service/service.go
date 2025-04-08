package service

import (
	"fmt"
	"reflect"
	"time"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/pkg/errors"
)

// Logger defines the logging interface for WorkflowService
type Logger interface {
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// TaskResult represents the output of a task or flow
type TaskResult interface{}

// TaskFunc is a function defining task/flow, with dependencies as parameters
type TaskFunc interface{}

type WorkflowService struct {
	store   storage.Store
	logger  Logger
	tasks   map[string]TaskFunc
	flows   map[string]TaskFunc
	results map[int64]map[string]TaskResult
}

func NewWorkflowService(store storage.Store, logger Logger) *WorkflowService {
	return &WorkflowService{
		store:   store,
		logger:  logger,
		tasks:   make(map[string]TaskFunc),
		flows:   make(map[string]TaskFunc),
		results: make(map[int64]map[string]TaskResult)}
}

// RegisterTask registers a task, inferring dependencies from parameter names
func (s *WorkflowService) RegisterTask(name string, fn TaskFunc) error {
	if err := validateTaskFunc(fn); err != nil {
		return fmt.Errorf("invalid task function for '%s': %v", name, err)
	}
	s.tasks[name] = fn
	s.logger.Infof("Registered task '%s'", name)
	return nil
}

// RegisterFlow registers a flow, inferring dependencies from parameter names
func (s *WorkflowService) RegisterFlow(name string, fn TaskFunc) error {
	if err := validateTaskFunc(fn); err != nil {
		return fmt.Errorf("invalid flow function for '%s': %v", name, err)
	}
	s.flows[name] = fn
	s.logger.Infof("Registered flow '%s'", name)
	return nil
}

// validateTaskFunc checks if the function matches the expected signature
func validateTaskFunc(fn TaskFunc) error {
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		return errors.New("must be a function")
	}
	// Ensure 2 returns: first is any type (TaskResult), second is error
	if fnType.NumOut() != 2 || fnType.Out(0).Kind() == reflect.Invalid || fnType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		return errors.New("must return (TaskResult, error)")
	}
	// Note: fnType.Out(0) isn’t checked against TaskResult (interface{}) explicitly,
	// as any type satisfies it. This allows flexibility (e.g., string, int).
	return nil
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
				s.logger.Errorf("Failed to rollback after error: %v (original error: %v)", rollbackErr, err)
			}
			return
		}
		if commitErr := txStore.Commit(); commitErr != nil {
			s.logger.Errorf("Failed to commit: %v", commitErr)
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
	s.logger.Infof("Created workflow '%s' with ID %d", name, id)
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
				s.logger.Errorf("Failed to rollback after error: %v (original error: %v)", rollbackErr, err)
			}
			return
		}
		if commitErr := txStore.Commit(); commitErr != nil {
			s.logger.Errorf("Failed to commit: %v", commitErr)
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

	s.logger.Infof("Updated workflow ID %d to status '%s'", id, status)
	return nil
}

func (s *WorkflowService) ListWorkflows() ([]models.Workflow, error) {
	return s.store.ListWorkflows()
}
