package storage

import (
	"github.com/ignatij/goflow/pkg/models"
	"github.com/pkg/errors"
)

var (
	ErrNotFound = errors.New("resource not found")
)

// Store defines the storage operations for GoFlow.
type Store interface {
	// SaveWorkflow creates a new workflow and returns its ID.
	SaveWorkflow(w models.Workflow) (int64, error)
	// GetWorkflow retrieves a workflow by ID, returning ErrNotFound if absent.
	GetWorkflow(id int64) (models.Workflow, error)
	// UpdateWorkflowStatus updates a workflow status by its ID.
	UpdateWorkflowStatus(id int64, status models.WorkflowStatus) error
	// ListWorkflows retrieves the list of all workflows.
	ListWorkflows() ([]models.Workflow, error)

	// SaveTask saves task.
	SaveTask(t models.Task) error
	// GetTask retrieves a task by its ID, returning ErrNotFound if absent.
	GetTask(id string, workflowID int64) (models.Task, error)
	// UpdateTaskStatus updates a task status by its ID.
	UpdateTaskStatus(id string, workflowID int64, status models.TaskStatus, errorMsg string) error

	// Begin starts a new transaction.
	Begin() (Store, error)
	// Commit commits already started transaction. Returning an error if the underlying store is not already a transaction.
	Commit() error
	// Rollback rollbacks already started transaction. Returning an error if the underlying store is not already a transaction.
	Rollback() error
	// Close closes the DB connection.
	Close() error
}
