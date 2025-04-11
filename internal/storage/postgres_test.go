package storage_test

import (
	"testing"
	"time"

	internal_storage "github.com/ignatij/goflow/internal/storage"
	"github.com/ignatij/goflow/internal/testutil"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestPostgresStore(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Teardown(t)

	// Helper to create a transactional store
	newTxStore := func(t *testing.T) *internal_storage.PostgresStore {
		store, err := internal_storage.NewPostgresStore(testDB.ConnStr)
		assert.NoError(t, err)
		txStore, err := store.Begin()
		assert.NoError(t, err)
		t.Cleanup(func() { txStore.Rollback() })
		return txStore.(*internal_storage.PostgresStore)
	}

	// Test SaveWorkflow
	t.Run("SaveWorkflow", func(t *testing.T) {
		store := newTxStore(t)
		wf := models.Workflow{
			Name:      "TestWorkflow",
			Status:    "PENDING",
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
	})

	// Test GetWorkflow
	t.Run("GetWorkflow", func(t *testing.T) {
		store := newTxStore(t)
		wf := models.Workflow{
			Name:      "GetTestWorkflow",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		wfID, err := store.SaveWorkflow(wf)
		assert.NoError(t, err)

		task0 := models.Task{ID: "t0", WorkflowID: wfID, Name: "Task0", Status: "PENDING"}
		err = store.SaveTask(task0)
		assert.NoError(t, err)
		task1 := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "PENDING"}
		err = store.SaveTask(task1)
		assert.NoError(t, err)

		retrieved, err := store.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, wf.Name, retrieved.Name)
		assert.Len(t, retrieved.Tasks, 2)
		assert.Equal(t, "t1", retrieved.Tasks[1].ID)
	})

	// GetNonExistingWorkflow
	t.Run("GetNonExistingWorkflow", func(t *testing.T) {
		store := newTxStore(t)
		_, err := store.GetWorkflow(123)
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	// Test UpdateWorkflowStatus
	t.Run("UpdateWorkflowStatus", func(t *testing.T) {
		store := newTxStore(t)
		wf := models.Workflow{
			Name:      "UpdateStatusTest",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		wfID, err := store.SaveWorkflow(wf)
		assert.NoError(t, err)

		err = store.UpdateWorkflowStatus(wfID, models.RunningWorkflowStatus)
		assert.NoError(t, err)

		updated, err := store.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.RunningWorkflowStatus, updated.Status)
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
			Status:    "PENDING",
			CreatedAt: time.Now().Add(-2 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		}
		wf2 := models.Workflow{
			Name:      "Workflow 2",
			Status:    "RUNNING",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		}
		wf3 := models.Workflow{
			Name:      "Workflow 3",
			Status:    "COMPLETED",
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
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{
			ID:         "t1",
			WorkflowID: wfID,
			Name:       "Task1",
			Status:     "PENDING",
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
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "PENDING"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		retrieved, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, "Task1", retrieved.Name)
	})

	// Test GetNonExistingTask
	t.Run("GetNonExistingTask", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "GetTaskTest",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		_, err = store.GetTask("t1", wfID)
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	// Test UpdateTaskStatus
	t.Run("UpdateTaskStatus", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "UpdateTaskTest",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{ID: "t1", WorkflowID: wfID, Name: "Task1", Status: "PENDING"}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		err = store.UpdateTaskStatus("t1", wfID, "COMPLETED", "All good")
		assert.NoError(t, err)

		updated, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, "COMPLETED", updated.Status)
		assert.Equal(t, "All good", updated.ErrorMsg)
		assert.NotNil(t, updated.FinishedAt)
	})
}
