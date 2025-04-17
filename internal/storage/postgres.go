package storage

import (
	"database/sql"
	"fmt"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type DBInterface interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
	QueryRowx(query string, args ...interface{}) *sqlx.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}

type PostgresStore struct {
	db DBInterface
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

func (s *PostgresStore) Commit() error {
	if tx, ok := s.db.(*sqlx.Tx); ok {
		return tx.Commit()
	}
	return fmt.Errorf("cannot commit: not a transaction")
}

func (s *PostgresStore) Rollback() error {
	if tx, ok := s.db.(*sqlx.Tx); ok {
		return tx.Rollback()
	}
	return fmt.Errorf("cannot rollback: not a transaction")
}

func (s *PostgresStore) Close() error {
	if db, ok := s.db.(*sqlx.DB); ok {
		return db.Close()
	}
	return nil // No-op for *sqlx.Tx
}

func (s *PostgresStore) SaveWorkflow(w models.Workflow) (int64, error) {
	var wfID int64
	err := s.db.QueryRowx("INSERT INTO workflows (name, status, created_at, updated_at) VALUES ($1, $2, $3, $4) RETURNING id",
		w.Name, w.Status, w.CreatedAt, w.UpdatedAt).Scan(&wfID)
	if err != nil {
		return 0, fmt.Errorf("save workflow: %w", err)
	}
	return wfID, nil
}

func (s *PostgresStore) GetWorkflow(id int64) (models.Workflow, error) {
	var wf models.Workflow
	err := s.db.Get(&wf, "SELECT * FROM workflows WHERE id = $1", id)
	if err == sql.ErrNoRows {
		return models.Workflow{}, storage.ErrNotFound
	}
	if err != nil {
		return models.Workflow{}, err
	}

	// Fetch tasks
	err = s.db.Select(&wf.Tasks, "SELECT * FROM tasks WHERE workflow_id = $1 ORDER BY id", id)
	if err != nil {
		return models.Workflow{}, fmt.Errorf("get workflow %d tasks: %w", id, err)
	}

	return wf, nil
}

func (s *PostgresStore) ListWorkflows() ([]models.Workflow, error) {
	workflows := []models.Workflow{}
	query := "SELECT id, name, status, created_at, updated_at FROM workflows ORDER BY created_at DESC"
	err := s.db.Select(&workflows, query)
	if err != nil {
		return nil, err
	}
	return workflows, nil
}

func (s *PostgresStore) UpdateWorkflowStatus(id int64, status models.WorkflowStatus) error {
	_, err := s.db.Exec("UPDATE workflows SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2", status, id)
	return err
}

func (s *PostgresStore) SaveTask(t models.Task) error {
	_, err := s.db.Exec(`
		INSERT INTO tasks (id, workflow_id, name, status, retries, attempts, error_msg, started_at, finished_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id, workflow_id) DO NOTHING`,
		t.ID, t.WorkflowID, t.Name, t.Status, t.Retries, t.Attempts, t.ErrorMsg, t.StartedAt, t.FinishedAt)
	return err
}

func (s *PostgresStore) GetTask(id string, workflowID int64) (models.Task, error) {
	var task models.Task
	err := s.db.Get(&task, "SELECT * FROM tasks WHERE id = $1 AND workflow_id = $2", id, workflowID)
	if err == sql.ErrNoRows {
		return models.Task{}, storage.ErrNotFound
	}
	if err != nil {
		return models.Task{}, err
	}
	return task, nil
}

func (s *PostgresStore) UpdateTaskStatus(id string, workflowID int64, status, errorMsg string) error {
	_, err := s.db.Exec(`
		UPDATE tasks 
		SET status = $1, 
		error_msg = $2, 
		finished_at = CASE WHEN $3 IN ('COMPLETED', 'FAILED') THEN CURRENT_TIMESTAMP ELSE finished_at END
		WHERE id = $4 AND workflow_id = $5`,
		status, errorMsg, status, id, workflowID)
	return err
}
