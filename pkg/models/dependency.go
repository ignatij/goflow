package models

// Dependency defines a relationship where one task depends on another.
type Dependency struct {
	TaskID     string `json:"task_id" db:"task_id"`         // Task that depends on another
	DependsOn  string `json:"depends_on" db:"depends_on"`   // Prerequisite task
	WorkflowID int    `json:"workflow_id" db:"workflow_id"` // Foreign key to Workflow
}
