package models

import "time"

// Workflow represents a collection of tasks and their dependencies.
type Workflow struct {
	ID           int                 `json:"id" db:"id"`                 // Unique identifier (PostgreSQL auto-increment)
	Name         string              `json:"name" db:"name"`             // Descriptive name (e.g., "DataPipeline")
	Status       string              `json:"status" db:"status"`         // "pending", "running", "completed", "failed"
	CreatedAt    time.Time           `json:"created_at" db:"created_at"` // Creation timestamp
	UpdatedAt    time.Time           `json:"updated_at" db:"updated_at"` // Last update timestamp
	Tasks        []Task              `json:"tasks,omitempty"`            // Tasks in the workflow (populated at runtime)
	Dependencies map[string][]string `json:"deps,omitempty"`             // Task ID -> list of prerequisite task IDs
}
