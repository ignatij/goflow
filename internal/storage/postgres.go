package storage

import (
	"database/sql"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type PostgresStore struct {
	db *sqlx.DB
}

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

// SaveWorkflow creates a new workflow and returns its ID (no tasks/deps)
func (s *PostgresStore) SaveWorkflow(w models.Workflow) (int, error) {
	var wfID int
	err := s.db.QueryRowx("INSERT INTO workflows (name, status, created_at, updated_at) VALUES ($1, $2, $3, $4) RETURNING id",
		w.Name, w.Status, w.CreatedAt, w.UpdatedAt).Scan(&wfID)
	if err != nil {
		return 0, err
	}
	return wfID, nil
}

// GetWorkflow retrieves a workflow by ID, including tasks and dependencies
func (s *PostgresStore) GetWorkflow(id int) (models.Workflow, error) {
	var wf models.Workflow
	err := s.db.Get(&wf, "SELECT * FROM workflows WHERE id = $1", id)
	if err == sql.ErrNoRows {
		return models.Workflow{}, sql.ErrNoRows
	}
	if err != nil {
		return models.Workflow{}, err
	}

	// Fetch tasks
	err = s.db.Select(&wf.Tasks, "SELECT * FROM tasks WHERE workflow_id = $1 ORDER BY id", id)
	if err != nil {
		return models.Workflow{}, err
	}

	// Fetch dependencies
	var deps []models.Dependency
	err = s.db.Select(&deps, "SELECT task_id, depends_on FROM dependencies WHERE workflow_id = $1", id)
	if err != nil {
		return models.Workflow{}, err
	}
	wf.Dependencies = make(map[string][]string)
	for _, dep := range deps {
		wf.Dependencies[dep.TaskID] = append(wf.Dependencies[dep.TaskID], dep.DependsOn)
	}

	return wf, nil
}

// UpdateWorkflowStatus updates the status of a workflow
func (s *PostgresStore) UpdateWorkflowStatus(id int, status string) error {
	_, err := s.db.Exec("UPDATE workflows SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2", status, id)
	return err
}

// SaveTask creates a new task within a workflow
func (s *PostgresStore) SaveTask(t models.Task) error {
	_, err := s.db.Exec("INSERT INTO tasks (id, workflow_id, name, status, retries, attempts, error_msg, started_at, finished_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		t.ID, t.WorkflowID, t.Name, t.Status, t.Retries, t.Attempts, t.ErrorMsg, t.StartedAt, t.FinishedAt)
	return err
}

// GetTask retrieves a task by ID and workflow ID
func (s *PostgresStore) GetTask(id string, workflowID int) (models.Task, error) {
	var task models.Task
	err := s.db.Get(&task, "SELECT * FROM tasks WHERE id = $1 AND workflow_id = $2", id, workflowID)
	if err == sql.ErrNoRows {
		return models.Task{}, sql.ErrNoRows
	}
	if err != nil {
		return models.Task{}, err
	}
	return task, nil
}

// UpdateTaskStatus updates the status and error message of a task
func (s *PostgresStore) UpdateTaskStatus(id string, workflowID int, status, errorMsg string) error {
	_, err := s.db.Exec(`
		UPDATE tasks 
		SET status = $1, 
		error_msg = $2, 
		finished_at = CASE WHEN $3 IN ('completed', 'failed') THEN CURRENT_TIMESTAMP ELSE finished_at END
		WHERE id = $4 AND workflow_id = $5`,
		// PostgreSQL interprets the parameters in the CASE clause of your SQL query as separate so passing the status twice
		status, errorMsg, status, id, workflowID)
	return err
}

// SaveDependency creates a new dependency between tasks
func (s *PostgresStore) SaveDependency(d models.Dependency) error {
	_, err := s.db.Exec(`
		INSERT INTO dependencies (task_id, depends_on, workflow_id) VALUES ($1, $2, $3)
		`,
		d.TaskID, d.DependsOn, d.WorkflowID)
	return err
}

// GetDependencies retrieves all dependencies for a workflow
func (s *PostgresStore) GetDependencies(workflowID int) ([]models.Dependency, error) {
	var deps []models.Dependency
	err := s.db.Select(&deps, "SELECT task_id, depends_on, workflow_id FROM dependencies WHERE workflow_id = $1", workflowID)
	if err != nil {
		return nil, err
	}
	return deps, nil
}
