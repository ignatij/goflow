package models

import "time"

type WorkflowStatus string

const (
	PendingWorkflowStatus   WorkflowStatus = "PENDING"
	RunningWorkflowStatus   WorkflowStatus = "RUNNING"
	CompletedWorkflowStatus WorkflowStatus = "COMPLETED"
	FailedWorkflowStatus    WorkflowStatus = "FAILED"
)

// Workflow represents a collection of tasks and their dependencies.
type Workflow struct {
	ID        int64          `json:"id" db:"id"`                 // Unique identifier (PostgreSQL auto-increment)
	Name      string         `json:"name" db:"name"`             // Descriptive name (e.g., "DataPipeline")
	Status    WorkflowStatus `json:"status" db:"status"`         // "pending", "running", "completed", "failed"
	CreatedAt time.Time      `json:"created_at" db:"created_at"` // Creation timestamp
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"` // Last update timestamp
	Tasks     []Task         `json:"tasks,omitempty"`            // Tasks in the workflow (populated at runtime)
}
