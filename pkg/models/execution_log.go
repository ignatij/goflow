package models

import "time"

// ExecutionLog tracks the history of task executions for auditing.
type ExecutionLog struct {
	ID         int       `json:"id" db:"id"`                     // Auto-incremented log ID
	TaskID     string    `json:"task_id" db:"task_id"`           // Task being logged
	WorkflowID int       `json:"workflow_id" db:"workflow_id"`   // Parent workflow
	Status     string    `json:"status" db:"status"`             // Status at this point
	Message    string    `json:"message,omitempty" db:"message"` // Details (e.g., error or success note)
	LoggedAt   time.Time `json:"logged_at" db:"logged_at"`       // Timestamp of log entry
}
