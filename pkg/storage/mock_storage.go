package storage

import (
	"time"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/pkg/errors"
)

// mockStore implements storage.Store with in-memory storage
type mockStore struct {
	workflows    []models.Workflow
	tasks        []models.Task
	dependencies []models.Dependency
	nextID       int64 // For workflow IDs
	committed    bool  // Transaction state
}

func (m *mockStore) Begin() (Store, error) {
	return m, nil
}

func (m *mockStore) ListWorkflows() ([]models.Workflow, error) {
	return m.workflows, nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) SaveWorkflow(wf models.Workflow) (int64, error) {
	if m.committed {
		return 0, errors.New("transaction already committed")
	}
	m.nextID++
	wf.ID = m.nextID
	m.workflows = append(m.workflows, wf)
	return wf.ID, nil
}

func (m *mockStore) GetWorkflow(id int64) (models.Workflow, error) {
	for _, wf := range m.workflows {
		if wf.ID == id {
			return wf, nil
		}
	}
	return models.Workflow{}, errors.New("workflow not found")
}

func (m *mockStore) UpdateWorkflowStatus(id int64, status models.WorkflowStatus) error {
	if m.committed {
		return errors.New("transaction already committed")
	}
	for i, wf := range m.workflows {
		if wf.ID == id {
			m.workflows[i].Status = status
			m.workflows[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return errors.New("workflow not found")
}

func (m *mockStore) SaveTask(t models.Task) error {
	if m.committed {
		return errors.New("transaction already committed")
	}
	// Check for duplicate task ID within the same workflow
	for _, existing := range m.tasks {
		if existing.ID == t.ID && existing.WorkflowID == t.WorkflowID {
			return errors.New("task already exists")
		}
	}
	m.tasks = append(m.tasks, t)
	return nil
}

func (m *mockStore) GetTask(id string, workflowID int64) (models.Task, error) {
	for _, t := range m.tasks {
		if t.ID == id && t.WorkflowID == workflowID {
			return t, nil
		}
	}
	return models.Task{}, errors.New("task not found")
}

func (m *mockStore) UpdateTaskStatus(id string, workflowID int64, status, errorMsg string) error {
	if m.committed {
		return errors.New("transaction already committed")
	}
	for i, t := range m.tasks {
		if t.ID == id && t.WorkflowID == workflowID {
			m.tasks[i].Status = status
			m.tasks[i].ErrorMsg = errorMsg
			m.tasks[i].FinishedAt = &time.Time{} // Set to now; adjust as needed
			return nil
		}
	}
	return errors.New("task not found")
}

func (m *mockStore) SaveDependency(d models.Dependency) error {
	if m.committed {
		return errors.New("transaction already committed")
	}
	// Check for duplicate dependency
	for _, existing := range m.dependencies {
		if existing.TaskID == d.TaskID && existing.DependsOn == d.DependsOn && existing.WorkflowID == d.WorkflowID {
			return errors.New("dependency already exists")
		}
	}
	m.dependencies = append(m.dependencies, d)
	return nil
}

func (m *mockStore) GetDependencies(workflowID int64) ([]models.Dependency, error) {
	var deps []models.Dependency
	for _, d := range m.dependencies {
		if d.WorkflowID == workflowID {
			deps = append(deps, d)
		}
	}
	return deps, nil
}

func (m *mockStore) Commit() error {
	if m.committed {
		return errors.New("already committed")
	}
	m.committed = true
	return nil
}

func (m *mockStore) Rollback() error {
	if m.committed {
		return errors.New("cannot rollback committed transaction")
	}
	// No-op: changes are discarded when transaction ends (new instance from Begin)
	return nil
}

func NewMockStore() Store {
	return &mockStore{}
}
