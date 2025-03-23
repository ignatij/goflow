package storage

import (
	"github.com/ignatij/goflow/pkg/models"
)

// Store defines the storage operations for GoFlow.
type Store interface {
	SaveWorkflow(w models.Workflow) (int64, error)
	GetWorkflow(id int64) (models.Workflow, error)
	UpdateWorkflowStatus(id int64, status string) error
	ListWorkflows() ([]models.Workflow, error)

	SaveTask(t models.Task) error
	GetTask(id string, workflowID int64) (models.Task, error)
	UpdateTaskStatus(id string, workflowID int64, status, errorMsg string) error

	SaveDependency(d models.Dependency) error
	GetDependencies(workflowID int64) ([]models.Dependency, error)

	Begin() (Store, error)
	Commit() error
	Rollback() error
	Close() error
}
