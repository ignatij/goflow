package storage

import (
	"time"

	"github.com/ignatij/goflow/pkg/models"
)

// mockStore implements storage.Store with in-memory storage
// mockStore implements storage.Store for testing
type mockStore struct {
	workflows map[int64]models.Workflow
	tasks     map[int64][]models.Task
	nextID    int64
}

func NewMockStore() *mockStore {
	return &mockStore{
		workflows: make(map[int64]models.Workflow),
		tasks:     make(map[int64][]models.Task),
		nextID:    1,
	}
}

func (m *mockStore) Begin() (Store, error) { return m, nil }
func (m *mockStore) Commit() error         { return nil }
func (m *mockStore) Rollback() error       { return nil }
func (m *mockStore) Close() error          { return nil }

func (m *mockStore) SaveWorkflow(wf models.Workflow) (int64, error) {
	wf.ID = m.nextID
	m.nextID++
	m.workflows[wf.ID] = wf
	return wf.ID, nil
}

func (m *mockStore) GetWorkflow(id int64) (models.Workflow, error) {
	wf, ok := m.workflows[id]
	if !ok {
		return models.Workflow{}, ErrNotFound
	}
	result := wf
	result.Tasks = m.tasks[id]
	return result, nil
}

func (m *mockStore) ListWorkflows() ([]models.Workflow, error) {
	var wfs []models.Workflow
	for _, wf := range m.workflows {
		wfs = append(wfs, wf)
	}
	return wfs, nil
}

func (m *mockStore) UpdateWorkflowStatus(id int64, status models.WorkflowStatus) error {
	wf, ok := m.workflows[id]
	if !ok {
		return ErrNotFound
	}
	wf.Status = status
	wf.UpdatedAt = time.Now()
	m.workflows[id] = wf
	return nil
}

func (m *mockStore) SaveTask(t models.Task) error {
	// Check if task already exists for this workflowID
	for _, existing := range m.tasks[t.WorkflowID] {
		if existing.ID == t.ID {
			return nil // Task already exists, skip appending
		}
	}
	// New task, append it
	m.tasks[t.WorkflowID] = append(m.tasks[t.WorkflowID], t)
	return nil
}

func (m *mockStore) GetTask(id string, workflowID int64) (models.Task, error) {
	for _, t := range m.tasks[workflowID] {
		if t.ID == id {
			return t, nil
		}
	}
	return models.Task{}, ErrNotFound
}

func (m *mockStore) UpdateTaskStatus(id string, workflowID int64, status, errorMsg string) error {
	tasks := m.tasks[workflowID]
	for i, t := range tasks {
		if t.ID == id {
			t.Status = status
			t.ErrorMsg = errorMsg
			if status == "COMPLETED" || status == "FAILED" {
				now := time.Now()
				t.FinishedAt = &now
			}
			tasks[i] = t
			m.tasks[workflowID] = tasks
			return nil
		}
	}
	return ErrNotFound
}

func (m *mockStore) SaveDependency(d models.Dependency) error { return nil }
func (m *mockStore) GetDependencies(workflowID int64) ([]models.Dependency, error) {
	return []models.Dependency{}, nil
}
