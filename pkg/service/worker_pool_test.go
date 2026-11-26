package service_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
)

// testLogger implements Logger interface for testing
type testLogger struct {
}

func newLogger(t *testing.T) service.Logger {
	return &testLogger{}
}

func (l *testLogger) Infof(format string, args ...interface{}) {
}

func (l *testLogger) Errorf(format string, args ...interface{}) {
}

// NewTestTaskService creates a new TaskService for testing
func NewTestTaskService(store storage.Store) *service.TaskService {
	return service.NewTaskService(store, newLogger(nil))
}

// NewTestInMemoryStore creates a new in-memory store for testing
func NewTestInMemoryStore() storage.Store {
	return storage.NewMockStore()
}

func TestWorkerPool_TaskExecution(t *testing.T) {

	// Setup
	store := NewTestInMemoryStore()
	taskService := NewTestTaskService(store)
	logger := newLogger(t)
	ctx := context.Background()
	attempts := 0
	// Test cases
	tests := []struct {
		name           string
		taskFn         service.ContextTaskFunc
		timeout        time.Duration
		retries        int
		contextTimeout time.Duration
		expectedStatus models.TaskStatus
		expectedError  string
	}{
		{
			name: "Successful task execution",
			taskFn: func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
				return "success", nil
			},
			timeout:        5 * time.Second,
			retries:        0,
			contextTimeout: 10 * time.Second,
			expectedStatus: models.CompletedTaskStatus,
			expectedError:  "",
		},
		{
			name: "Task timeout",
			taskFn: func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
				time.Sleep(2 * time.Second)
				return nil, nil
			},
			timeout:        1 * time.Second,
			retries:        0,
			contextTimeout: 10 * time.Second,
			expectedStatus: models.FailedTaskStatus,
			expectedError:  "context deadline exceeded",
		},
		{
			name: "Task cancellation",
			taskFn: func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
				// Sleep for a short time to allow cancellation to take effect
				time.Sleep(100 * time.Millisecond)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					return nil, nil
				}
			},
			timeout:        5 * time.Second,
			retries:        0,
			contextTimeout: 10 * time.Second,
			expectedStatus: models.FailedTaskStatus,
			expectedError:  "context canceled",
		},
		{
			name: "Task retry success",
			taskFn: func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
				// Fail on first attempt, succeed on retry
				if attempts == 0 {
					attempts++
					return nil, fmt.Errorf("temporary error")
				}
				return "success after retry", nil
			},
			timeout:        5 * time.Second,
			retries:        1,
			contextTimeout: 10 * time.Second,
			expectedStatus: models.CompletedTaskStatus,
			expectedError:  "",
		},
		{
			name: "Task retry failure",
			taskFn: func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
				return nil, fmt.Errorf("permanent error")
			},
			timeout:        5 * time.Second,
			retries:        1,
			contextTimeout: 10 * time.Second,
			expectedStatus: models.FailedTaskStatus,
			expectedError:  "permanent error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create worker pool
			wp := service.NewWorkerPool(
				ctx,
				map[string]service.ContextTaskFunc{"test_task": tt.taskFn},
				map[string][]string{"test_task": {}},
				store,
				taskService,
				logger,
			)
			wp.Start(1)

			// Cleanup worker pool at the end of the test
			defer wp.Stop()

			// Create task config
			taskConfig := &models.TaskConfig{
				Timeout: &tt.timeout,
				Retries: tt.retries,
			}
			wp.UpdateTasks(
				map[string]service.ContextTaskFunc{"test_task": tt.taskFn},
				map[string][]string{"test_task": {}},
				map[string]string{"test_task": "task"},
				map[string]*models.TaskConfig{"test_task": taskConfig},
			)

			// Create workflow context
			workflowCtx := service.WorkflowContext{
				WorkflowID:  1,
				Results:     make(map[string]service.TaskResult),
				ResultsLock: &sync.RWMutex{},
			}

			// Create context with timeout
			execCtx, cancel := context.WithTimeout(ctx, tt.contextTimeout)
			defer cancel()

			// Generate consistent execution ID
			execID := fmt.Sprintf("exec-%d-test_task", workflowCtx.WorkflowID)

			// Save initial task state
			task := models.Task{
				ID:           "test_task",
				WorkflowID:   workflowCtx.WorkflowID,
				Name:         "test_task",
				Status:       models.PendingTaskStatus,
				ExecutionID:  execID,
				Dependencies: []string{},
				Retries:      tt.retries,
				Timeout:      &tt.timeout,
				Attempts:     0,
			}
			if err := store.SaveTask(task); err != nil {
				t.Fatalf("Failed to save initial task state: %v", err)
			}

			// For cancellation test, cancel the context after a short delay
			if tt.name == "Task cancellation" {
				go func() {
					time.Sleep(50 * time.Millisecond)
					cancel()
				}()
			}

			// Execute task
			results, errors := wp.ExecuteTasks(execCtx, execID, workflowCtx, []string{"test_task"})

			// Verify results
			if tt.expectedError == "" {
				if len(errors) > 0 {
					t.Errorf("Expected no errors, got: %v", errors)
				}
				if len(results) == 0 {
					t.Error("Expected results, got none")
				}
			} else {
				if len(errors) == 0 {
					t.Error("Expected error, got none")
				} else {
					err := errors["test_task"]
					if err == nil {
						err = errors[execID]
					}
					if err == nil || !strings.Contains(err.Error(), tt.expectedError) {
						t.Errorf("Expected error containing %q, got: %v", tt.expectedError, err)
					}
				}
			}

			// Verify task status in store
			task, err := store.GetTask("test_task", 1)
			if err != nil {
				t.Errorf("Failed to get task: %v", err)
			}
			if task.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, task.Status)
			}
			if tt.retries != 0 && task.Attempts-1 != tt.retries {
				t.Errorf("Expected attempts %d, got %d", tt.retries, task.Attempts-1)
			}
		})
	}
}

func TestWorkerPool_DependentTasks(t *testing.T) {
	// Setup
	store := NewTestInMemoryStore()
	taskService := NewTestTaskService(store)
	logger := newLogger(t)
	ctx := context.Background()

	// Create worker pool
	tasks := map[string]service.ContextTaskFunc{
		"slow": func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			time.Sleep(2 * time.Second)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return "slow completed", nil
			}
		},
		"pipeline": func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			if len(args) == 0 {
				return nil, fmt.Errorf("no dependencies provided")
			}
			return "pipeline completed", nil
		},
	}
	taskDeps := map[string][]string{
		"slow":     {},
		"pipeline": {"slow"},
	}
	taskTypes := map[string]string{
		"slow":     "task",
		"pipeline": "task",
	}
	wp := service.NewWorkerPool(
		ctx,
		tasks,
		taskDeps,
		store,
		taskService,
		logger,
	)
	wp.UpdateTasks(tasks, taskDeps, taskTypes, map[string]*models.TaskConfig{})
	wp.Start(1)

	// Cleanup worker pool at the end of the test
	defer wp.Stop()

	// Create workflow context
	workflowCtx := service.WorkflowContext{
		WorkflowID:  1,
		Results:     make(map[string]service.TaskResult),
		ResultsLock: &sync.RWMutex{},
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Generate consistent execution ID
	execID := fmt.Sprintf("exec-%d-pipeline", workflowCtx.WorkflowID)

	// Save initial task states
	slowTask := models.Task{
		ID:           "slow",
		WorkflowID:   workflowCtx.WorkflowID,
		Name:         "slow",
		Status:       models.PendingTaskStatus,
		ExecutionID:  execID,
		Dependencies: []string{},
		Retries:      0,
		Timeout:      nil,
		Attempts:     0,
	}
	if err := store.SaveTask(slowTask); err != nil {
		t.Fatalf("Failed to save slow task: %v", err)
	}

	pipelineTask := models.Task{
		ID:           "pipeline",
		WorkflowID:   workflowCtx.WorkflowID,
		Name:         "pipeline",
		Status:       models.PendingTaskStatus,
		ExecutionID:  execID,
		Dependencies: []string{"slow"},
		Retries:      0,
		Timeout:      nil,
		Attempts:     0,
	}
	if err := store.SaveTask(pipelineTask); err != nil {
		t.Fatalf("Failed to save pipeline task: %v", err)
	}

	// Execute tasks
	_, errors := wp.ExecuteTasks(execCtx, execID, workflowCtx, []string{"slow", "pipeline"})

	// Give some time for database updates to complete
	time.Sleep(100 * time.Millisecond)

	// Verify results
	if len(errors) == 0 {
		t.Error("Expected errors due to timeout, got none")
	} else {
		// Check that both tasks failed
		slowTask, err := store.GetTask("slow", 1)
		if err != nil {
			t.Errorf("Failed to get slow task: %v", err)
		}
		if slowTask.Status != models.FailedTaskStatus {
			t.Errorf("Expected slow task status FAILED, got %s", slowTask.Status)
		}

		pipelineTask, err := store.GetTask("pipeline", 1)
		if err != nil {
			t.Errorf("Failed to get pipeline task: %v", err)
		}
		if pipelineTask.Status != models.FailedTaskStatus {
			t.Errorf("Expected pipeline task status FAILED, got %s", pipelineTask.Status)
		}
	}
}

func TestWorkerPool_ExponentialBackoff(t *testing.T) {
	store := NewTestInMemoryStore()
	logger := newLogger(t)
	ctx := context.Background()
	svc := service.NewWorkflowService(ctx, store, logger)

	var attemptTimes []time.Time
	err := svc.RegisterTask(
		"flaky",
		func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			attemptTimes = append(attemptTimes, time.Now())
			if len(attemptTimes) < 4 {
				return nil, fmt.Errorf("transient failure %d", len(attemptTimes))
			}
			return "ok", nil
		},
		nil,
		models.WithRetries(3),
		models.WithRetryBackoff(40*time.Millisecond, 200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("RegisterTask failed: %v", err)
	}
	if err := svc.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		return args[0], nil
	}, []string{"flaky"}); err != nil {
		t.Fatalf("RegisterFlow failed: %v", err)
	}

	wfID, err := svc.CreateWorkflow("backoff-test")
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}

	res, err := svc.ExecuteFlow(ctx, wfID, "main")
	if err != nil {
		t.Fatalf("ExecuteFlow failed: %v", err)
	}
	if res != "ok" {
		t.Fatalf("expected 'ok', got %v", res)
	}
	if len(attemptTimes) != 4 {
		t.Fatalf("expected 4 attempts, got %d", len(attemptTimes))
	}

	// Equal jitter guarantees:
	// gap0 in [20ms, 40ms], gap1 in [40ms, 80ms], gap2 in [80ms, 160ms]
	// Total delay >= 140ms, and gap2 >= gap0 (80ms vs <=40ms).
	gap0 := attemptTimes[1].Sub(attemptTimes[0])
	gap2 := attemptTimes[3].Sub(attemptTimes[2])
	total := attemptTimes[3].Sub(attemptTimes[0])
	if total < 130*time.Millisecond {
		t.Errorf("expected total backoff >= 130ms, got %v (gap0=%v, gap2=%v)", total, gap0, gap2)
	}
	if gap2 <= gap0 {
		t.Errorf("expected exponential growth (gap2 > gap0), got gap0=%v gap2=%v", gap0, gap2)
	}
}

func TestWorkerPool_CancelDuringBackoff(t *testing.T) {
	store := NewTestInMemoryStore()
	logger := newLogger(t)
	svc := service.NewWorkflowService(context.Background(), store, logger)

	attempts := 0
	if err := svc.RegisterTask(
		"slow-backoff",
		func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
			attempts++
			return nil, fmt.Errorf("always fails")
		},
		nil,
		models.WithRetries(5),
		models.WithRetryBackoff(5*time.Second, 30*time.Second),
	); err != nil {
		t.Fatalf("RegisterTask failed: %v", err)
	}
	if err := svc.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		return args[0], nil
	}, []string{"slow-backoff"}); err != nil {
		t.Fatalf("RegisterFlow failed: %v", err)
	}

	wfID, err := svc.CreateWorkflow("cancel-backoff")
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}

	execCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = svc.ExecuteFlow(execCtx, wfID, "main")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when context cancels during backoff")
	}
	if elapsed > 1*time.Second {
		t.Fatalf("ExecuteFlow did not abort promptly during backoff sleep: took %v", elapsed)
	}
	if attempts != 1 {
		t.Errorf("expected only 1 attempt before cancel during 5s backoff, got %d", attempts)
	}
}
