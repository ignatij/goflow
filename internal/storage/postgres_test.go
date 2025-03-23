package storage

import (
	"testing"
	"time"

	"github.com/ignatij/goflow/internal/testutil"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestPostgresStore(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Teardown(t)

	// Helper to create a transactional store
	newTxStore := func(t *testing.T) *PostgresStore {
		store, err := InitStore(testDB.ConnStr)
		assert.NoError(t, err)
		txStore, err := store.Begin()
		assert.NoError(t, err)
		t.Cleanup(func() { txStore.Rollback() })
		return txStore.(*PostgresStore)
	}

	// Test SaveWorkflow
	t.Run("SaveWorkflow", func(t *testing.T) {
		store := newTxStore(t)
		wf := models.Workflow{
			Name:      "TestWorkflow",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		wfID, err := store.SaveWorkflow(wf)
		assert.NoError(t, err)
		assert.Greater(t, wfID, int64(0))

		savedWf, err := store.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, wf.Name, savedWf.Name)
		assert.Equal(t, wf.Status, savedWf.Status)
		assert.Empty(t, savedWf.Tasks)
		assert.Empty(t, savedWf.Dependencies)
	})

	// Test GetWorkflow
	t.Run("GetWorkflow", func(t *testing.T) {
		store := newTxStore(t)
		wf := models.Workflow{
			Name:      "GetTestWorkflow",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		wfID, err := store.SaveWorkflow(wf)
		assert.NoError(t, err)

		task0 := models.Task{ID: "t0", WorkflowID: wfID, Name: "Task0", Status: "pending"}
		err = store.SaveTask(task0)
		assert.NoError(t, err)
		task1 := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "pending"}
		err = store.SaveTask(task1)
		assert.NoError(t, err)
		dep := models.Dependency{TaskID: "t1", DependsOn: "t0", WorkflowID: wfID}
		err = store.SaveDependency(dep)
		assert.NoError(t, err)

		retrieved, err := store.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, wf.Name, retrieved.Name)
		assert.Len(t, retrieved.Tasks, 2)
		assert.Equal(t, "t1", retrieved.Tasks[1].ID)
		assert.Equal(t, []string{"t0"}, retrieved.Dependencies["t1"])
	})

	// Test UpdateWorkflowStatus
	t.Run("UpdateWorkflowStatus", func(t *testing.T) {
		store := newTxStore(t)
		wf := models.Workflow{
			Name:      "UpdateStatusTest",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		wfID, err := store.SaveWorkflow(wf)
		assert.NoError(t, err)

		err = store.UpdateWorkflowStatus(wfID, "running")
		assert.NoError(t, err)

		updated, err := store.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "running", updated.Status)
	})

	// Test ListWorkflows (Empty)
	t.Run("ListWorkflows returns empty list when no workflows exist", func(t *testing.T) {
		store := newTxStore(t)
		workflows, err := store.ListWorkflows()
		assert.NoError(t, err)
		assert.Empty(t, workflows)
	})

	// Test ListWorkflows (Populated)
	t.Run("ListWorkflows returns workflows in descending order", func(t *testing.T) {
		store := newTxStore(t)
		wf1 := models.Workflow{
			Name:      "Workflow 1",
			Status:    "pending",
			CreatedAt: time.Now().Add(-2 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		}
		wf2 := models.Workflow{
			Name:      "Workflow 2",
			Status:    "running",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		}
		wf3 := models.Workflow{
			Name:      "Workflow 3",
			Status:    "completed",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		id1, err := store.SaveWorkflow(wf1)
		assert.NoError(t, err)
		id2, err := store.SaveWorkflow(wf2)
		assert.NoError(t, err)
		id3, err := store.SaveWorkflow(wf3)
		assert.NoError(t, err)

		workflows, err := store.ListWorkflows()
		assert.NoError(t, err)
		assert.Len(t, workflows, 3)
		assert.Equal(t, id3, workflows[0].ID)
		assert.Equal(t, "Workflow 3", workflows[0].Name)
		assert.Equal(t, id2, workflows[1].ID)
		assert.Equal(t, "Workflow 2", workflows[1].Name)
		assert.Equal(t, id1, workflows[2].ID)
		assert.Equal(t, "Workflow 1", workflows[2].Name)
	})

	// Test SaveTask
	t.Run("SaveTask", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "TaskTestWorkflow",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{
			ID:         "t1",
			WorkflowID: wfID,
			Name:       "Task1",
			Status:     "pending",
			Retries:    2,
		}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		savedTask, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, task.Name, savedTask.Name)
		assert.Equal(t, task.Retries, savedTask.Retries)
	})

	// Test GetTask
	t.Run("GetTask", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "GetTaskTest",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "pending"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		retrieved, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, "Task1", retrieved.Name)
	})

	// Test UpdateTaskStatus
	t.Run("UpdateTaskStatus", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "UpdateTaskTest",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "pending"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		err = store.UpdateTaskStatus("t1", wfID, "completed", "All good")
		assert.NoError(t, err)

		updated, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, "completed", updated.Status)
		assert.Equal(t, "All good", updated.ErrorMsg)
		assert.NotNil(t, updated.FinishedAt)
	})

	// Test SaveDependency
	t.Run("SaveDependency", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "DepTestWorkflow",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "pending"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		task = models.Task{ID: "t2", WorkflowID: wfID, Name: "Task2", Status: "pending"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		dep := models.Dependency{TaskID: "t2", DependsOn: "t1", WorkflowID: wfID}
		err = store.SaveDependency(dep)
		assert.NoError(t, err)

		deps, err := store.GetDependencies(wfID)
		assert.NoError(t, err)
		assert.Len(t, deps, 1)
		assert.Equal(t, "t2", deps[0].TaskID)
		assert.Equal(t, "t1", deps[0].DependsOn)
	})

	// Test GetDependencies
	t.Run("GetDependencies", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "GetDepsTest",
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "finished"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		task = models.Task{ID: "t2", WorkflowID: wfID, Name: "Task2", Status: "finished"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		task = models.Task{ID: "t3", WorkflowID: wfID, Name: "Task3", Status: "pending"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		deps := []models.Dependency{
			{TaskID: "t2", DependsOn: "t1", WorkflowID: wfID},
			{TaskID: "t3", DependsOn: "t2", WorkflowID: wfID},
		}
		for _, d := range deps {
			err = store.SaveDependency(d)
			assert.NoError(t, err)
		}

		retrieved, err := store.GetDependencies(wfID)
		assert.NoError(t, err)
		assert.Len(t, retrieved, 2)
		assert.Contains(t, retrieved, models.Dependency{TaskID: "t2", DependsOn: "t1", WorkflowID: wfID})
		assert.Contains(t, retrieved, models.Dependency{TaskID: "t3", DependsOn: "t2", WorkflowID: wfID})
	})
}
