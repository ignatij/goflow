package models

import "time"

// Task represents a single executable unit within a workflow.
type Task struct {
	ID         string       `json:"id" db:"id"`                             // Unique identifier (e.g., "t1" or UUID)
	WorkflowID int          `json:"workflow_id" db:"workflow_id"`           // Foreign key to Workflow
	Name       string       `json:"name" db:"name"`                         // Descriptive name (e.g., "FetchData")
	Status     string       `json:"status" db:"status"`                     // "pending", "running", "completed", "failed"
	Retries    int          `json:"retries" db:"retries"`                   // Max retry attempts
	Attempts   int          `json:"attempts" db:"attempts"`                 // Current attempt count
	ErrorMsg   string       `json:"error,omitempty" db:"error_msg"`         // Last error message (optional)
	StartedAt  *time.Time   `json:"started_at,omitempty" db:"started_at"`   // Nullable start time
	FinishedAt *time.Time   `json:"finished_at,omitempty" db:"finished_at"` // Nullable end time
	Run        func() error `json:"-"`                                      // Execution logic (runtime only, not persisted)
}
