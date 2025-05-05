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

		task0 := models.Task{
			ID:           "t0",
			WorkflowID:   wfID,
			Name:         "Task0",
			Status:       "PENDING",
			ExecutionID:  "wf1:flow1",
			Dependencies: nil,
		}
		err = store.SaveTask(task0)
		assert.NoError(t, err)
		task1 := models.Task{
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			ExecutionID:  "wf1:flow1",
			Dependencies: []string{"t0"},
		}
		err = store.SaveTask(task1)
		assert.NoError(t, err)

		retrieved, err := store.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, wf.Name, retrieved.Name)
		assert.Len(t, retrieved.Tasks, 2)
		assert.Equal(t, "t0", retrieved.Tasks[0].ID)
		assert.Equal(t, "wf1:flow1", retrieved.Tasks[0].ExecutionID)
		assert.Equal(t, []string{}, retrieved.Tasks[0].Dependencies)
		assert.Equal(t, "t1", retrieved.Tasks[1].ID)
		assert.Equal(t, "wf1:flow1", retrieved.Tasks[1].ExecutionID)
		assert.Equal(t, []string{"t0"}, retrieved.Tasks[1].Dependencies)
	})

	// Test GetNonExistingWorkflow
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
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			Retries:      2,
			ExecutionID:  "wf1:flow1",
			Dependencies: []string{"t0"},
		}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		savedTask, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, task.Name, savedTask.Name)
		assert.Equal(t, task.Retries, savedTask.Retries)
		assert.Equal(t, task.ExecutionID, savedTask.ExecutionID)
		assert.Equal(t, task.Dependencies, savedTask.Dependencies)
	})

	// Test SaveTaskIdempotent
	t.Run("SaveTaskIdempotent", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "IdempotentTaskTest",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			Retries:      2,
			ExecutionID:  "wf1:flow1",
			Dependencies: []string{"t0"},
		}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		// Save again with same ID and WorkflowID but different ExecutionID and Dependencies
		task2 := models.Task{
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			Retries:      2,
			ExecutionID:  "wf1:flow2",
			Dependencies: []string{"t0", "t2"},
		}
		err = store.SaveTask(task2)
		assert.NoError(t, err)

		// Verify the task is updated to task2
		savedTask, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, task2.ExecutionID, savedTask.ExecutionID)
		assert.Equal(t, task2.Dependencies, savedTask.Dependencies)
	})

	// Test SaveTaskWithNilDependencies
	t.Run("SaveTaskWithNilDependencies", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "NilDependenciesTest",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			Retries:      2,
			ExecutionID:  "wf1:flow1",
			Dependencies: nil,
		}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		savedTask, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, task.Name, savedTask.Name)
		assert.Equal(t, task.Retries, savedTask.Retries)
		assert.Equal(t, task.ExecutionID, savedTask.ExecutionID)
		assert.Equal(t, []string{}, savedTask.Dependencies)
	})

	// Test SaveTaskWithEmptyDependencies
	t.Run("SaveTaskWithEmptyDependencies", func(t *testing.T) {
		store := newTxStore(t)
		wfID, err := store.SaveWorkflow(models.Workflow{
			Name:      "EmptyDependenciesTest",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		assert.NoError(t, err)

		task := models.Task{
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			Retries:      2,
			ExecutionID:  "wf1:flow1",
			Dependencies: []string{},
		}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		savedTask, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, task.Name, savedTask.Name)
		assert.Equal(t, task.Retries, savedTask.Retries)
		assert.Equal(t, task.ExecutionID, savedTask.ExecutionID)
		assert.Equal(t, []string{}, savedTask.Dependencies)
	})

	// Test Dependencies
	t.Run("Dependencies", func(t *testing.T) {
		store := newTxStore(t)
		wf := models.Workflow{
			Name:      "DependenciesTest",
			Status:    "PENDING",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		wfID, err := store.SaveWorkflow(wf)
		assert.NoError(t, err)

		// Define tasks with varied Dependencies
		tasks := []models.Task{
			{
				ID:           "t0",
				WorkflowID:   wfID,
				Name:         "Task0",
				Status:       "PENDING",
				ExecutionID:  "wf1:flow1",
				Dependencies: nil,
			},
			{
				ID:           "t1",
				WorkflowID:   wfID,
				Name:         "Task1",
				Status:       "PENDING",
				ExecutionID:  "wf1:flow1",
				Dependencies: []string{},
			},
			{
				ID:           "t2",
				WorkflowID:   wfID,
				Name:         "Task2",
				Status:       "PENDING",
				ExecutionID:  "wf1:flow1",
				Dependencies: []string{"t0"},
			},
			{
				ID:           "t3",
				WorkflowID:   wfID,
				Name:         "Task3",
				Status:       "PENDING",
				ExecutionID:  "wf1:flow1",
				Dependencies: []string{"t0", "t1", "t2"},
			},
			{
				ID:           "t4",
				WorkflowID:   wfID,
				Name:         "Task4",
				Status:       "PENDING",
				ExecutionID:  "wf1:flow1",
				Dependencies: []string{"t0,t1", `t2"t3`, ""},
			},
		}

		// Save all tasks
		for _, task := range tasks {
			err := store.SaveTask(task)
			assert.NoError(t, err)
		}

		// Verify tasks via GetTask
		for _, expected := range tasks {
			retrieved, err := store.GetTask(expected.ID, wfID)
			assert.NoError(t, err)
			if expected.Dependencies == nil {
				assert.Equal(t, []string{}, retrieved.Dependencies)
			} else {
				// Filter out empty strings for comparison
				filtered := []string{}
				for _, dep := range expected.Dependencies {
					if dep != "" {
						filtered = append(filtered, dep)
					}
				}
				assert.Equal(t, filtered, retrieved.Dependencies)
			}
			assert.Equal(t, expected.Name, retrieved.Name)
			assert.Equal(t, expected.ExecutionID, retrieved.ExecutionID)
		}

		// Verify tasks via GetWorkflow
		retrievedWf, err := store.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, wf.Name, retrievedWf.Name)
		assert.Len(t, retrievedWf.Tasks, len(tasks))
		for i, expected := range tasks {
			assert.Equal(t, expected.ID, retrievedWf.Tasks[i].ID)
			if expected.Dependencies == nil {
				assert.Equal(t, []string{}, retrievedWf.Tasks[i].Dependencies)
			} else {
				// Filter out empty strings
				filtered := []string{}
				for _, dep := range expected.Dependencies {
					if dep != "" {
						filtered = append(filtered, dep)
					}
				}
				assert.Equal(t, filtered, retrievedWf.Tasks[i].Dependencies)
			}
			assert.Equal(t, expected.Name, retrievedWf.Tasks[i].Name)
			assert.Equal(t, expected.ExecutionID, retrievedWf.Tasks[i].ExecutionID)
		}
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

		task := models.Task{
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			ExecutionID:  "wf1:flow1",
			Dependencies: []string{"t0"},
		}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		retrieved, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, task.Name, retrieved.Name)
		assert.Equal(t, task.ExecutionID, retrieved.ExecutionID)
		assert.Equal(t, task.Dependencies, retrieved.Dependencies)
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

		task := models.Task{
			ID:           "t1",
			WorkflowID:   wfID,
			Name:         "Task1",
			Status:       "PENDING",
			ExecutionID:  "wf1:flow1",
			Dependencies: []string{"t0"},
		}
		err = store.SaveTask(task)
		assert.NoError(t, err)

		err = store.UpdateTaskStatus("t1", wfID, "COMPLETED", "All good")
		assert.NoError(t, err)

		updated, err := store.GetTask("t1", wfID)
		assert.NoError(t, err)
		assert.Equal(t, "COMPLETED", updated.Status)
		assert.Equal(t, "All good", updated.ErrorMsg)
		assert.NotNil(t, updated.FinishedAt)
		assert.Equal(t, task.ExecutionID, updated.ExecutionID)
		assert.Equal(t, task.Dependencies, updated.Dependencies)
	})
}
