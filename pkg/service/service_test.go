package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	internal_storage "github.com/ignatij/goflow/internal/storage"
	"github.com/ignatij/goflow/internal/testutil"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type logger struct{}

func (l logger) Infof(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}

func (l logger) Errorf(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
}

// Define a custom type for context keys
type contextKey string

const (
	testKey contextKey = "test_key"
)

func TestClientWorkflowInMemory_Pipeline(t *testing.T) {

	newWorkflowService := func() *service.WorkflowService {
		return service.NewWorkflowService(context.Background(), storage.NewMockStore(), logger{})
	}

	t.Run("UnregisteredFlow", func(t *testing.T) {
		svc := newWorkflowService()
		wfID, err := svc.CreateWorkflow("test")
		assert.NoError(t, err)
		_, err = svc.ExecuteFlow(context.Background(), wfID, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "flow 'nonexistent' is not registered!")
	})

	t.Run("UnregisteredTaskInFlow", func(t *testing.T) {
		svc := newWorkflowService()

		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		// Register flow with non-existent task dependency
		err := svc.RegisterFlow("pipeline", pipelineFlow, []string{"nonexistent"})
		assert.NoError(t, err)

		wfID, err := svc.CreateWorkflow("testUnregisteredTask")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency 'nonexistent' for 'pipeline' not registered")
		assert.Nil(t, result)

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testUnregisteredTask", wf.Name)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 0)
	})

	t.Run("DuplicateTaskRegistration", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask1 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data 1", nil
		}
		fetchTask2 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data 2", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			str, ok := data[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", data)
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask1, []string{}))
		assert.NoError(t, svc.RegisterTask("fetch", fetchTask2, []string{})) // Duplicate, no error
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		wfID, err := svc.CreateWorkflow("testDuplicateTask")
		assert.NoError(t, err)
		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: raw data 2", result) // Uses fetchTask2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateTask", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1) // Single task persisted due to SaveTask logic
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, models.CompletedTaskStatus, wf.Tasks[0].Status)
	})

	t.Run("DuplicateFlowRegistration", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow1 := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed v1: " + str, nil
		}
		pipelineFlow2 := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed v2: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow1, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow2, []string{"fetch"})) // Duplicate, no error

		wfID, err := svc.CreateWorkflow("testDuplicateFlow")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed v2: raw data", result) // Uses pipelineFlow2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateFlow", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1)
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, models.CompletedTaskStatus, wf.Tasks[0].Status)
	})

	t.Run("InvalidTaskDependencies", func(t *testing.T) {
		svc := newWorkflowService()

		processTask := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"nonexistent"}))
		pipelineFlow := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			return args[0], nil
		}
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("testInvalidDeps")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency 'nonexistent' for 'process' not registered")
		assert.Nil(t, result)

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testInvalidDeps", wf.Name)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 0)
	})

	t.Run("EmptyTaskFlowRegistration", func(t *testing.T) {
		svc := newWorkflowService()

		err := svc.RegisterTask("", func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "data", nil
		}, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty task name")

		err = svc.RegisterFlow("", func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "data", nil
		}, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty flow name")

		err = svc.RegisterTask("fetch", nil, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a function")

		err = svc.RegisterFlow("pipeline", nil, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a function")
	})
	t.Run("BasicPipeline", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			str, ok := data[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", data)
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: raw data", result)

		workflows, err := svc.ListWorkflows()
		assert.NoError(t, err)
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "dataPipeline", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, workflows[0].Status)

		// Verify tasks in DB
		assert.Len(t, wf.Tasks, 1) // Only "fetch" task persisted
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, models.CompletedTaskStatus, wf.Tasks[0].Status)
	})

	t.Run("MoreComplexPipeline", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		processTask := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			return "processing: " + args[0].(string), nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: processing: raw data", result)

		workflows, err := svc.ListWorkflows()
		assert.NoError(t, err)
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "dataPipeline", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, workflows[0].Status)

		// Verify tasks in DB
		assert.Len(t, wf.Tasks, 2)
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "process", wf.Tasks[1].ID)
		assert.Equal(t, models.CompletedTaskStatus, wf.Tasks[0].Status)
		assert.Equal(t, models.CompletedTaskStatus, wf.Tasks[1].Status)
	})

	t.Run("MultipleDependencies", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		processTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}
		pipelineFlow := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			fetched := args[0]
			processed := args[1]
			return fetched.(string) + " | " + processed.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch", "process"}))

		wfID, err := svc.CreateWorkflow("multiDepPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "raw data | processed: raw data", result)

		workflows, err := svc.ListWorkflows()
		assert.NoError(t, err)
		assert.Len(t, workflows, 1)
		assert.Equal(t, "multiDepPipeline", workflows[0].Name)
		assert.Equal(t, models.CompletedWorkflowStatus, workflows[0].Status)

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Len(t, wf.Tasks, 2)
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, models.CompletedTaskStatus, wf.Tasks[0].Status)
		assert.Equal(t, "process", wf.Tasks[1].ID)
		assert.Equal(t, models.CompletedTaskStatus, wf.Tasks[1].Status)
	})

	t.Run("MultipleFlows", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		cleanTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "cleaned: " + data[0].(string), nil
		}
		preprocessFlow := func(ctx context.Context, cleaned ...service.TaskResult) (service.TaskResult, error) {
			return cleaned[0], nil
		}
		analyzeTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "analyzed: " + data[0].(string), nil
		}
		reportFlow := func(ctx context.Context, analyzed ...service.TaskResult) (service.TaskResult, error) {
			return "report: " + analyzed[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("clean", cleanTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("preprocess", preprocessFlow, []string{"clean"}))
		assert.NoError(t, svc.RegisterTask("analyze", analyzeTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("report", reportFlow, []string{"analyze"}))

		wfID, err := svc.CreateWorkflow("multiFlowPipeline")
		assert.NoError(t, err)

		// Execute preprocess flow
		preprocessResult, err := svc.ExecuteFlow(context.Background(), wfID, "preprocess")
		assert.NoError(t, err)
		assert.Equal(t, "cleaned: raw data", preprocessResult)

		// Execute report flow on the same workflow
		reportResult, err := svc.ExecuteFlow(context.Background(), wfID, "report")
		assert.NoError(t, err)
		assert.Equal(t, "report: analyzed: raw data", reportResult)

		// Verify workflow status and tasks
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 3) // "fetch", "clean", "analyze" (fetch reused)
		for _, task := range wf.Tasks {
			assert.Equal(t, models.CompletedTaskStatus, task.Status)
		}
		assert.Contains(t, []string{"fetch", "clean", "analyze"}, wf.Tasks[0].ID)
		assert.Contains(t, []string{"fetch", "clean", "analyze"}, wf.Tasks[1].ID)
		assert.Contains(t, []string{"fetch", "clean", "analyze"}, wf.Tasks[2].ID)
	})

	t.Run("FailureHandling", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "data", nil
		}
		failTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return nil, errors.New("task failed")
		}
		flow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return data, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("fail", failTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("testFlow", flow, []string{"fail"}))

		wfID, err := svc.CreateWorkflow("failTest")
		assert.NoError(t, err)

		_, err = svc.ExecuteFlow(context.Background(), wfID, "testFlow")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "execution of flow 'testFlow' failed")

		// Verify workflow and task status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 2)
		for _, task := range wf.Tasks {
			if task.ID == "fetch" {
				assert.Equal(t, "COMPLETED", string(task.Status))
			}
			if task.ID == "fail" {
				assert.Equal(t, "FAILED", string(task.Status))
				assert.Equal(t, "task failed", task.ErrorMsg)
			}
		}
	})

	t.Run("MultipleRuns", func(t *testing.T) {
		svc := newWorkflowService()
		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return time.Now().String(), nil // Dynamic result
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		// First run
		wfID1, err := svc.CreateWorkflow("run1")
		assert.NoError(t, err)
		result1, err := svc.ExecuteFlow(context.Background(), wfID1, "pipeline")
		assert.NoError(t, err)

		// Second run
		wfID2, err := svc.CreateWorkflow("run2")
		assert.NoError(t, err)
		result2, err := svc.ExecuteFlow(context.Background(), wfID2, "pipeline")
		assert.NoError(t, err)

		// Results should differ due to time-based fetch
		assert.NotEqual(t, result1, result2)

		// Verify both workflows
		workflows, err := svc.ListWorkflows()
		assert.NoError(t, err)
		assert.Len(t, workflows, 2)
		for _, wf := range workflows {
			assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		}

		wf1, err := svc.GetWorkflow(wfID1)
		assert.NoError(t, err)
		assert.Len(t, wf1.Tasks, 1)
		assert.Equal(t, "fetch", wf1.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", string(wf1.Tasks[0].Status))

		wf2, err := svc.GetWorkflow(wfID2)
		assert.NoError(t, err)
		assert.Len(t, wf2.Tasks, 1)
		assert.Equal(t, "fetch", wf2.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", string(wf2.Tasks[0].Status))
	})

	t.Run("ParallelTaskExecution", func(t *testing.T) {
		svc := newWorkflowService()
		task1 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(100 * time.Millisecond)
			return "result1", nil
		}
		task2 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(100 * time.Millisecond)
			return "result2", nil
		}

		pipelineFlow := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			result := make([]string, 0, len(args))
			for i := 0; i < len(args); i++ {
				result = append(result, args[i].(string))
			}
			return strings.Join(result, " | "), nil
		}
		assert.NoError(t, svc.RegisterTask("task1", task1, []string{}))
		assert.NoError(t, svc.RegisterTask("task2", task2, []string{}))
		assert.NoError(t, svc.RegisterFlow("parallelPipeline", pipelineFlow, []string{"task1", "task2"}))
		wfID, err := svc.CreateWorkflow("parallelTest")
		assert.NoError(t, err)
		start := time.Now()
		result, err := svc.ExecuteFlow(context.Background(), wfID, "parallelPipeline")
		duration := time.Since(start)
		assert.NoError(t, err)
		assert.Equal(t, "result1 | result2", result)
		// assert less then 150 miliseconds which proves the asynchronius way
		assert.Less(t, duration, 150*time.Millisecond)
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 2)
		taskIDs := make(map[string]bool)
		for _, task := range wf.Tasks {
			taskIDs[task.ID] = true
			assert.Equal(t, models.CompletedTaskStatus, task.Status)
		}
		assert.True(t, taskIDs["task1"])
		assert.True(t, taskIDs["task2"])
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		svc := newWorkflowService()

		slowTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(500 * time.Millisecond)
			return "slow result", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("slow", slowTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"slow"}))

		wfID, err := svc.CreateWorkflow("cancelTest")
		assert.NoError(t, err)

		// Create a context that will be cancelled after 100ms
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err = svc.ExecuteFlow(ctx, wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")

		// Verify workflow status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
	})

	t.Run("ContextWithTimeout", func(t *testing.T) {
		svc := newWorkflowService()

		slowTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(200 * time.Millisecond)
			return "slow result", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("slow", slowTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"slow"}))

		wfID, err := svc.CreateWorkflow("timeoutTest")
		assert.NoError(t, err)

		// Create a context with a 1s timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		result, err := svc.ExecuteFlow(ctx, wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: slow result", result)

		// Verify workflow status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
	})

	t.Run("ContextWithValues", func(t *testing.T) {
		svc := newWorkflowService()

		// Create a context with a value
		ctx := context.WithValue(context.Background(), testKey, "test_value")

		taskWithContext := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			value := ctx.Value(testKey).(string)
			return "context value: " + value, nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return data[0], nil
		}

		assert.NoError(t, svc.RegisterTask("context_task", taskWithContext, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"context_task"}))

		wfID, err := svc.CreateWorkflow("contextTest")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(ctx, wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "context value: test_value", result)

		// Verify workflow status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
	})

	t.Run("CancelledMainContext", func(t *testing.T) {
		// Create a context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())
		svc := newWorkflowService()

		// Create a slow task that checks context cancellation
		slowTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(1 * time.Second):
				return "slow result", nil
			}
		}

		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("slow", slowTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"slow"}))

		wfID, err := svc.CreateWorkflow("cancelTest")
		assert.NoError(t, err)

		// Start execution in a goroutine
		errChan := make(chan error, 1)
		go func() {
			_, err := svc.ExecuteFlow(ctx, wfID, "pipeline")
			errChan <- err
		}()

		// Wait a bit and then cancel
		time.Sleep(100 * time.Millisecond)
		t.Log("Cancelling context...")
		cancel()
		t.Log("Context cancelled")

		// Wait for either error or timeout
		select {
		case err := <-errChan:
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "context canceled")
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out - possible deadlock")
		}

		// Verify workflow status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
	})
}

func TestClientWorkflowPostgres_Pipelines(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Teardown(t)

	postgresStore := func(t *testing.T) storage.Store {
		store, err := internal_storage.InitStore(testDB.ConnStr)
		assert.NoError(t, err)
		t.Cleanup(func() {
			_, err := testDB.DB.Exec("TRUNCATE TABLE workflows RESTART IDENTITY CASCADE")
			assert.NoError(t, err)
			store.Close()
		})
		return store
	}

	t.Run("UnregisteredFlow", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})
		wfID, err := svc.CreateWorkflow("test")
		assert.NoError(t, err)
		_, err = svc.ExecuteFlow(context.Background(), wfID, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "flow 'nonexistent' is not registered!")
	})

	t.Run("UnregisteredTaskInFlow", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		// Register flow with non-existent task dependency
		err := svc.RegisterFlow("pipeline", pipelineFlow, []string{"nonexistent"})
		assert.NoError(t, err)

		wfID, err := svc.CreateWorkflow("testUnregisteredTask")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency 'nonexistent' for 'pipeline' not registered")
		assert.Nil(t, result)

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testUnregisteredTask", wf.Name)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 0)
	})

	t.Run("DuplicateTaskRegistration", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		fetchTask1 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data 1", nil
		}
		fetchTask2 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data 2", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			str, ok := data[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", data)
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask1, []string{}))
		assert.NoError(t, svc.RegisterTask("fetch", fetchTask2, []string{})) // Duplicate, no error
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		wfID, err := svc.CreateWorkflow("testDuplicateTask")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: raw data 2", result) // Uses fetchTask2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateTask", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1) // Single task persisted due to SaveTask logic
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", string(wf.Tasks[0].Status))
	})

	t.Run("DuplicateFlowRegistration", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow1 := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed v1: " + str, nil
		}
		pipelineFlow2 := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed v2: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow1, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow2, []string{"fetch"})) // Duplicate, no error

		wfID, err := svc.CreateWorkflow("testDuplicateFlow")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed v2: raw data", result) // Uses pipelineFlow2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateFlow", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1)
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", string(wf.Tasks[0].Status))
	})

	t.Run("InvalidTaskDependencies", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		processTask := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"nonexistent"}))
		pipelineFlow := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			return args[0], nil
		}
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("testInvalidDeps")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency 'nonexistent' for 'process' not registered")
		assert.Nil(t, result)

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testInvalidDeps", wf.Name)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 0)
	})

	t.Run("EmptyTaskFlowRegistration", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		err := svc.RegisterTask("", func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "data", nil
		}, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty task name")

		err = svc.RegisterFlow("", func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "data", nil
		}, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty flow name")

		err = svc.RegisterTask("fetch", nil, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a function")

		err = svc.RegisterFlow("pipeline", nil, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a function")
	})

	t.Run("BasicPipeline", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: raw data", result)

		workflows, err := svc.ListWorkflows()
		assert.NoError(t, err)
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "dataPipeline", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, workflows[0].Status)

		// Verify tasks in DB
		assert.Len(t, wf.Tasks, 1) // Only "fetch" task persisted
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", string(wf.Tasks[0].Status))
	})

	t.Run("MoreComplexPipeline", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		processTask := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			return "processing: " + args[0].(string), nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: processing: raw data", result)

		workflows, err := svc.ListWorkflows()
		assert.NoError(t, err)
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "dataPipeline", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, workflows[0].Status)

		// Verify tasks in DB
		assert.Len(t, wf.Tasks, 2)
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "process", wf.Tasks[1].ID)
		assert.Equal(t, "COMPLETED", string(wf.Tasks[0].Status))
		assert.Equal(t, "COMPLETED", string(wf.Tasks[1].Status))
	})

	t.Run("MultipleFlows", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})
		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "raw data", nil
		}
		cleanTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "cleaned: " + data[0].(string), nil
		}
		preprocessFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return data[0], nil
		}
		analyzeTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "analyzed: " + data[0].(string), nil
		}
		reportFlow := func(ctx context.Context, analyzed ...service.TaskResult) (service.TaskResult, error) {
			return "report: " + analyzed[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("clean", cleanTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("preprocess", preprocessFlow, []string{"clean"}))
		assert.NoError(t, svc.RegisterTask("analyze", analyzeTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("report", reportFlow, []string{"analyze"}))

		wfID, err := svc.CreateWorkflow("multiFlowPipeline")
		assert.NoError(t, err)

		// Execute preprocess flow
		preprocessResult, err := svc.ExecuteFlow(context.Background(), wfID, "preprocess")
		assert.NoError(t, err)
		assert.Equal(t, "cleaned: raw data", preprocessResult)

		// Execute report flow on the same workflow
		reportResult, err := svc.ExecuteFlow(context.Background(), wfID, "report")
		assert.NoError(t, err)
		assert.Equal(t, "report: analyzed: raw data", reportResult)

		// Verify workflow status and tasks
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 3)
		taskIDs := make(map[string]bool)
		for _, task := range wf.Tasks {
			assert.Equal(t, "COMPLETED", string(task.Status))
			taskIDs[task.ID] = true
		}
		assert.Len(t, taskIDs, 3, "Expected exactly 3 unique tasks")
		assert.True(t, taskIDs["fetch"], "Expected fetch task")
		assert.True(t, taskIDs["clean"], "Expected clean task")
		assert.True(t, taskIDs["analyze"], "Expected analyze task")
	})

	t.Run("FailureHandling", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})
		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "data", nil
		}
		failTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return nil, errors.New("task failed")
		}
		flow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return data, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("fail", failTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("testFlow", flow, []string{"fail"}))

		wfID, err := svc.CreateWorkflow("failTest")
		assert.NoError(t, err)

		_, err = svc.ExecuteFlow(context.Background(), wfID, "testFlow")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "execution of flow 'testFlow' failed")

		// Verify workflow and task status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 2)
		for _, task := range wf.Tasks {
			if task.ID == "fetch" {
				assert.Equal(t, "COMPLETED", string(task.Status))
			}
			if task.ID == "fail" {
				assert.Equal(t, "FAILED", string(task.Status))
				assert.Equal(t, "task failed", task.ErrorMsg)
			}
		}
	})

	t.Run("ParallelTaskExecution", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})
		task1 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(100 * time.Millisecond)
			return "result1", nil
		}
		task2 := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(100 * time.Millisecond)
			return "result2", nil
		}

		pipelineFlow := func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			result := make([]string, 0, len(args))
			for i := 0; i < len(args); i++ {
				result = append(result, args[i].(string))
			}
			return strings.Join(result, " | "), nil
		}
		assert.NoError(t, svc.RegisterTask("task1", task1, []string{}))
		assert.NoError(t, svc.RegisterTask("task2", task2, []string{}))
		assert.NoError(t, svc.RegisterFlow("parallelPipeline", pipelineFlow, []string{"task1", "task2"}))
		wfID, err := svc.CreateWorkflow("parallelTest")
		assert.NoError(t, err)
		start := time.Now()
		result, err := svc.ExecuteFlow(context.Background(), wfID, "parallelPipeline")
		duration := time.Since(start)
		assert.NoError(t, err)
		assert.Equal(t, "result1 | result2", result)
		// assert less then 200 miliseconds which proves the asynchronius way
		assert.Less(t, duration, 200*time.Millisecond)
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 2)
		taskIDs := make(map[string]bool)
		for _, task := range wf.Tasks {
			taskIDs[task.ID] = true
			assert.Equal(t, models.CompletedTaskStatus, task.Status)
		}
		assert.True(t, taskIDs["task1"])
		assert.True(t, taskIDs["task2"])
	})

	t.Run("TaskRetries", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		attempts := 0
		fetchTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.Errorf("ooops, something went wrong on attempt: %d", attempts)
			}
			return "raw data", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}, models.WithRetries(2)))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: raw data", result)

		workflows, err := svc.ListWorkflows()
		assert.NoError(t, err)
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "dataPipeline", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, workflows[0].Status)

		// Verify tasks in DB
		assert.Len(t, wf.Tasks, 1) // Only "fetch" task persisted
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", string(wf.Tasks[0].Status))
		assert.Equal(t, 2, wf.Tasks[0].Retries)
		assert.Equal(t, 3, attempts)
	})

	t.Run("TaskTimeout", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		// This task intentionally sleeps for longer than the timeout
		slowTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			select {
			case <-time.After(2 * time.Second):
				return "this won't be returned", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		// Register task with a 1 second timeout
		assert.NoError(t, svc.RegisterTask("slow", slowTask, []string{}, models.WithTimeout(1*time.Second)))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"slow"}))

		wfID, err := svc.CreateWorkflow("timeoutPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")

		// Expect an error due to task timeout
		assert.Error(t, err)
		assert.Nil(t, result)

		// Check workflow and task status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)

		assert.Len(t, wf.Tasks, 1)
		assert.Equal(t, "slow", wf.Tasks[0].ID)
		assert.Equal(t, models.FailedTaskStatus, wf.Tasks[0].Status)
		assert.Contains(t, wf.Tasks[0].ErrorMsg, "context deadline exceeded")
	})

	t.Run("CancelledMainContext", func(t *testing.T) {
		// Create a context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())
		svc := service.NewWorkflowService(ctx, postgresStore(t), logger{})

		sleepTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			select {
			case <-time.After(5 * time.Second):
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed", nil
		}

		assert.NoError(t, svc.RegisterTask("sleep", sleepTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"sleep"}))

		wfID, err := svc.CreateWorkflow("cancelledContextPipeline")
		assert.NoError(t, err)

		// Start a goroutine to cancel the context after 1 second
		go func() {
			time.Sleep(1 * time.Second)
			cancel()
		}()

		result, err := svc.ExecuteFlow(context.Background(), wfID, "pipeline")

		// Expect an error due to context cancellation
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "context canceled")

		// Check workflow and task status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)

		assert.Len(t, wf.Tasks, 1)
		assert.Equal(t, "sleep", wf.Tasks[0].ID)
		assert.Equal(t, models.FailedTaskStatus, wf.Tasks[0].Status)
		assert.Contains(t, wf.Tasks[0].ErrorMsg, "context canceled")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		slowTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(500 * time.Millisecond)
			return "slow result", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("slow", slowTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"slow"}))

		wfID, err := svc.CreateWorkflow("cancelTest")
		assert.NoError(t, err)

		// Create a context that will be cancelled after 100ms
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err = svc.ExecuteFlow(ctx, wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")

		// Verify workflow status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
	})

	t.Run("ContextWithTimeout", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		slowTask := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(200 * time.Millisecond)
			return "slow result", nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data[0].(string), nil
		}

		assert.NoError(t, svc.RegisterTask("slow", slowTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"slow"}))

		wfID, err := svc.CreateWorkflow("timeoutTest")
		assert.NoError(t, err)

		// Create a context with a 1s timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		result, err := svc.ExecuteFlow(ctx, wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: slow result", result)

		// Verify workflow status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
	})

	t.Run("ContextWithValues", func(t *testing.T) {
		svc := service.NewWorkflowService(context.Background(), postgresStore(t), logger{})

		// Create a context with a value
		ctx := context.WithValue(context.Background(), testKey, "test_value")

		taskWithContext := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			value := ctx.Value(testKey).(string)
			return "context value: " + value, nil
		}
		pipelineFlow := func(ctx context.Context, data ...service.TaskResult) (service.TaskResult, error) {
			return data[0], nil
		}

		assert.NoError(t, svc.RegisterTask("context_task", taskWithContext, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"context_task"}))

		wfID, err := svc.CreateWorkflow("contextTest")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(ctx, wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "context value: test_value", result)

		// Verify workflow status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
	})

}

func TestClientWorkflowPostgres_Flows(t *testing.T) {
	// ... existing code ...
	// Remove the unused context variable
	// ... existing code ...
}
