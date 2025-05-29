package storage

import (
	"database/sql"
	"fmt"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/jmoiron/sqlx"

	_ "github.com/lib/pq"
)

// DBInterface defines the database operations
type DBInterface interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
	QueryRowx(query string, args ...interface{}) *sqlx.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// PostgresStore implements the Store interface for PostgreSQL
type PostgresStore struct {
	db DBInterface
}

// NewPostgresStore initializes a new PostgresStore
func NewPostgresStore(connStr string) (*PostgresStore, error) {
	db, err := sqlx.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

// Begin starts a transaction
func (s *PostgresStore) Begin() (storage.Store, error) {
	if db, ok := s.db.(*sqlx.DB); ok {
		tx, err := db.Beginx()
		if err != nil {
			return nil, err
		}
		return &PostgresStore{db: tx}, nil
	}
	return nil, fmt.Errorf("cannot begin transaction on unknown type")
}

// Commit commits a transaction
func (s *PostgresStore) Commit() error {
	if tx, ok := s.db.(*sqlx.Tx); ok {
		return tx.Commit()
	}
	return fmt.Errorf("cannot commit: not a transaction")
}

// Rollback rolls back a transaction
func (s *PostgresStore) Rollback() error {
	if tx, ok := s.db.(*sqlx.Tx); ok {
		return tx.Rollback()
	}
	return fmt.Errorf("cannot rollback: not a transaction")
}

// Close closes the database connection
func (s *PostgresStore) Close() error {
	if db, ok := s.db.(*sqlx.DB); ok {
		return db.Close()
	}
	return nil // No-op for *sqlx.Tx
}

// SaveWorkflow saves a workflow and returns its ID
func (s *PostgresStore) SaveWorkflow(w models.Workflow) (int64, error) {
	var wfID int64
	err := s.db.QueryRowx("INSERT INTO workflows (name, status, created_at, updated_at) VALUES ($1, $2, $3, $4) RETURNING id",
		w.Name, w.Status, w.CreatedAt, w.UpdatedAt).Scan(&wfID)
	if err != nil {
		return 0, fmt.Errorf("save workflow: %w", err)
	}
	return wfID, nil
}

// GetWorkflow retrieves a workflow by ID
func (s *PostgresStore) GetWorkflow(id int64) (models.Workflow, error) {
	var wf models.Workflow
	err := s.db.Get(&wf, "SELECT * FROM workflows WHERE id = $1", id)
	if err == sql.ErrNoRows {
		return models.Workflow{}, storage.ErrNotFound
	}
	if err != nil {
		return models.Workflow{}, err
	}

	// Fetch tasks without dependencies
	err = s.db.Select(&wf.Tasks, `
		SELECT 
			id, workflow_id, name, status, retries, attempts, error_msg, 
			started_at, finished_at, execution_id
		FROM tasks
		WHERE workflow_id = $1 
		ORDER BY id`, id)
	if err != nil {
		return models.Workflow{}, fmt.Errorf("get workflow %d tasks: %w", id, err)
	}

	// Fetch dependencies separately
	type dep struct {
		TaskID          string `db:"task_id"`
		DependsOnTaskID string `db:"depends_on_task_id"`
	}
	var deps []dep
	err = s.db.Select(&deps, `
		SELECT task_id, depends_on_task_id
		FROM task_dependencies
		WHERE workflow_id = $1
		ORDER BY task_id, depends_on_task_id`, id)
	if err != nil {
		return models.Workflow{}, fmt.Errorf("get workflow %d dependencies: %w", id, err)
	}

	// Map dependencies to tasks
	depMap := make(map[string][]string)
	for _, d := range deps {
		depMap[d.TaskID] = append(depMap[d.TaskID], d.DependsOnTaskID)
	}
	for i, task := range wf.Tasks {
		if deps, ok := depMap[task.ID]; ok {
			wf.Tasks[i].Dependencies = deps
		} else {
			wf.Tasks[i].Dependencies = []string{}
		}
		// Leave Dependencies as nil if no dependencies exist
	}

	return wf, nil
}

// ListWorkflows lists all workflows
func (s *PostgresStore) ListWorkflows() ([]models.Workflow, error) {
	workflows := []models.Workflow{}
	query := "SELECT id, name, status, created_at, updated_at FROM workflows ORDER BY created_at DESC"
	err := s.db.Select(&workflows, query)
	if err != nil {
		return nil, err
	}
	return workflows, nil
}

// UpdateWorkflowStatus updates a workflow's status
func (s *PostgresStore) UpdateWorkflowStatus(id int64, status models.WorkflowStatus) error {
	_, err := s.db.Exec("UPDATE workflows SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2", status, id)
	return err
}

// SaveTask saves a task
func (s *PostgresStore) SaveTask(t models.Task) error {
	// Save task without dependencies
	_, err := s.db.Exec(`
		INSERT INTO tasks (
			id, workflow_id, name, status, retries, attempts, error_msg, 
			started_at, finished_at, execution_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id, workflow_id) DO UPDATE SET
			name = EXCLUDED.name,
			status = EXCLUDED.status,
			retries = EXCLUDED.retries,
			attempts = EXCLUDED.attempts,
			error_msg = EXCLUDED.error_msg,
			started_at = EXCLUDED.started_at,
			finished_at = EXCLUDED.finished_at,
			execution_id = EXCLUDED.execution_id`,
		t.ID, t.WorkflowID, t.Name, t.Status, t.Retries, t.Attempts,
		t.ErrorMsg, t.StartedAt, t.FinishedAt, t.ExecutionID)
	if err != nil {
		return fmt.Errorf("save task %s: %w", t.ID, err)
	}

	// Delete existing dependencies
	_, err = s.db.Exec(`
		DELETE FROM task_dependencies 
		WHERE task_id = $1 AND workflow_id = $2`,
		t.ID, t.WorkflowID)
	if err != nil {
		return fmt.Errorf("delete dependencies for task %s: %w", t.ID, err)
	}

	// Insert new dependencies
	if t.Dependencies != nil {
		for _, depID := range t.Dependencies {
			if depID != "" && depID != t.ID { // Skip empty strings and self-references
				_, err = s.db.Exec(`
					INSERT INTO task_dependencies (task_id, workflow_id, depends_on_task_id)
					VALUES ($1, $2, $3)`,
					t.ID, t.WorkflowID, depID)
				if err != nil {
					return fmt.Errorf("save dependency %s for task %s: %w", depID, t.ID, err)
				}
			}
		}
	}

	return nil
}

// GetTask retrieves a task by ID and workflow ID
func (s *PostgresStore) GetTask(id string, workflowID int64) (models.Task, error) {
	var task models.Task
	err := s.db.Get(&task, `
		SELECT 
			id, workflow_id, name, status, retries, attempts, error_msg, 
			started_at, finished_at, execution_id
		FROM tasks
		WHERE id = $1 AND workflow_id = $2`, id, workflowID)
	if err == sql.ErrNoRows {
		return models.Task{}, storage.ErrNotFound
	}
	if err != nil {
		return models.Task{}, err
	}

	// Fetch dependencies
	deps := []string{}
	err = s.db.Select(&deps, `
		SELECT depends_on_task_id
		FROM task_dependencies
		WHERE task_id = $1 AND workflow_id = $2
		ORDER BY depends_on_task_id`, id, workflowID)
	if err != nil {
		return models.Task{}, fmt.Errorf("get task %s dependencies: %w", id, err)
	}
	task.Dependencies = deps

	return task, nil
}

// UpdateTaskStatus updates a task's status and error message
func (s *PostgresStore) UpdateTaskStatus(id string, workflowID int64, status models.TaskStatus, errorMsg string) error {
	_, err := s.db.Exec(`
		UPDATE tasks 
		SET status = $1, 
			error_msg = $2, 
			finished_at = CASE WHEN $3 IN ('COMPLETED', 'FAILED') THEN CURRENT_TIMESTAMP ELSE finished_at END
		WHERE id = $4 AND workflow_id = $5`,
		status, errorMsg, status, id, workflowID)
	return err
}
