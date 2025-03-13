package storage

import "github.com/ignatij/goflow/pkg/models"

// Store defines the storage operations for GoFlow.
type Store interface {
	// Workflow operations
	SaveWorkflow(w models.Workflow) (int, error)
	GetWorkflow(id int) (models.Workflow, error)
	UpdateWorkflowStatus(id int, status string) error

	// Task operations
	SaveTask(t models.Task) error
	GetTask(id string, workflowID int) (models.Task, error)
	UpdateTaskStatus(id string, workflowID int, status, errorMsg string) error

	// Dependency operations
	SaveDependency(d models.Dependency) error
	GetDependencies(workflowID int) ([]models.Dependency, error)
}
