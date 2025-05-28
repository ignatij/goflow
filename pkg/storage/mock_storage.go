package storage

import (
	"sync"
	"time"

	"github.com/ignatij/goflow/pkg/models"
)

// mockStore implements storage.Store with in-memory storage
// mockStore implements storage.Store for testing
type mockStore struct {
	workflows map[int64]models.Workflow
	tasks     map[int64][]models.Task
	nextID    int64
	mu        sync.RWMutex
}

// NewMockStore creates a new mock store
func NewMockStore() *mockStore {
	return &mockStore{
		workflows: make(map[int64]models.Workflow),
		tasks:     make(map[int64][]models.Task),
		nextID:    1,
		mu:        sync.RWMutex{},
	}
}

func (m *mockStore) Begin() (Store, error) {
	return m, nil
}
func (m *mockStore) Commit() error   { return nil }
func (m *mockStore) Rollback() error { return nil }
func (m *mockStore) Close() error    { return nil }

// SaveWorkflow saves a workflow and assigns an ID
func (m *mockStore) SaveWorkflow(wf models.Workflow) (int64, error) {
	m.mu.Lock()
	wf.ID = m.nextID
	m.nextID++
	m.workflows[wf.ID] = wf
	m.mu.Unlock()
	return wf.ID, nil
}

// GetWorkflow retrieves a workflow by ID
func (m *mockStore) GetWorkflow(id int64) (models.Workflow, error) {
	m.mu.RLock()
	wf, ok := m.workflows[id]
	m.mu.RUnlock()
	if !ok {
		return models.Workflow{}, ErrNotFound
	}
	result := wf
	m.mu.RLock()
	result.Tasks = m.tasks[id]
	m.mu.RUnlock()
	return result, nil
}

// ListWorkflows lists all workflows
func (m *mockStore) ListWorkflows() ([]models.Workflow, error) {
	var wfs []models.Workflow
	m.mu.RLock()
	for _, wf := range m.workflows {
		wfs = append(wfs, wf)
	}
	m.mu.RUnlock()
	return wfs, nil
}

// UpdateWorkflowStatus updates a workflow's status
func (m *mockStore) UpdateWorkflowStatus(id int64, status models.WorkflowStatus) error {
	m.mu.RLock()
	wf, ok := m.workflows[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	wf.Status = status
	wf.UpdatedAt = time.Now()
	m.mu.Lock()
	m.workflows[id] = wf
	m.mu.Unlock()
	return nil
}

// SaveTask saves a task
func (m *mockStore) SaveTask(t models.Task) error {
	// Check if task already exists for this workflowID and executionID
	m.mu.RLock()
	for _, existing := range m.tasks[t.WorkflowID] {
		if existing.ID == t.ID && existing.WorkflowID == t.WorkflowID {
			m.mu.RUnlock()
			return nil // Task already exists, skip appending
		}
	}
	m.mu.RUnlock()
	// New task, append it
	m.mu.Lock()
	m.tasks[t.WorkflowID] = append(m.tasks[t.WorkflowID], t)
	m.mu.Unlock()
	return nil
}

// GetTask retrieves a task by ID and workflow ID
func (m *mockStore) GetTask(id string, workflowID int64) (models.Task, error) {
	m.mu.RLock()
	for _, t := range m.tasks[workflowID] {
		if t.ID == id {
			m.mu.RUnlock()
			return t, nil
		}
	}
	m.mu.RUnlock()
	return models.Task{}, ErrNotFound
}

// UpdateTaskStatus updates a task's status and error message
func (m *mockStore) UpdateTaskStatus(id string, workflowID int64, status models.TaskStatus, errorMsg string) error {
	m.mu.Lock()
	tasks := m.tasks[workflowID]
	for i, t := range tasks {
		if t.ID == id && t.WorkflowID == workflowID {
			t.Status = status
			t.ErrorMsg = errorMsg
			if status == models.CompletedTaskStatus || status == models.FailedTaskStatus {
				now := time.Now()
				t.FinishedAt = &now
			}
			tasks[i] = t
			m.tasks[workflowID] = tasks
			m.mu.Unlock()
			return nil
		}
	}
	m.mu.Unlock()
	return ErrNotFound
}

// SaveDependency and GetDependencies are no-ops
func (m *mockStore) SaveDependency(d models.Dependency) error { return nil }
func (m *mockStore) GetDependencies(workflowID int64) ([]models.Dependency, error) {
	return []models.Dependency{}, nil
}
