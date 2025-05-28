package models

import "time"

type TaskStatus string

const (
	PendingTaskStatus   TaskStatus = "PENDING"
	RunningTaskStatus   TaskStatus = "RUNNING"
	FailedTaskStatus    TaskStatus = "FAILED"
	CompletedTaskStatus TaskStatus = "COMPLETED"
)

// Task represents a task in a workflow
type Task struct {
	ID           string     `json:"id" db:"id"`                             // Unique identifier (e.g., "t1" or UUID)
	WorkflowID   int64      `json:"workflow_id" db:"workflow_id"`           // Foreign key to Workflow
	Name         string     `json:"name" db:"name"`                         // Descriptive name (e.g., "FetchData")
	Status       TaskStatus `json:"status" db:"status"`                     // "pending", "running", "completed", "failed"
	Retries      int        `json:"retries" db:"retries"`                   // Max retry attempts
	Attempts     int        `json:"attempts" db:"attempts"`                 // Current attempt count
	ErrorMsg     string     `json:"error,omitempty" db:"error_msg"`         // Last error message (optional)
	StartedAt    *time.Time `json:"started_at,omitempty" db:"started_at"`   // Nullable start time
	FinishedAt   *time.Time `json:"finished_at,omitempty" db:"finished_at"` // Nullable end time
	ExecutionID  string     `json:"execution_id" db:"execution_id"`         // Unique execution identifier (e.g., "wf1:flow1")
	Dependencies []string   `json:"dependencies" db:"dependencies"`         // List of task IDs this task depends on
}
