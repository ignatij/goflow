package service

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
)

const (
	// default task timeout is 1m
	DefaultTaskTimeout = 60 * time.Second
)

// executionState holds state for a single execution (workflow + flow)
type executionState struct {
	taskErrors   map[string]error
	pendingCount int           // Tasks not yet completed or failed
	completeChan chan struct{} // Signals completion or error
	mu           sync.RWMutex
	cleanupOnce  sync.Once
}

type WorkflowContext struct {
	WorkflowID  int64
	Results     map[string]TaskResult // shared across all flows in workflow
	ResultsLock *sync.RWMutex         // protect access to shared results
}

type TaskContext struct {
	Task        *models.Task
	WorkflowCtx *WorkflowContext
	Ctx         context.Context
}

// WorkerPool manages parallel task execution with dependency enforcement
type WorkerPool struct {
	tasks       map[string]ContextTaskFunc
	taskDeps    map[string][]string
	taskTypes   map[string]string // can be "task" or "flow"
	taskConfigs map[string]*models.TaskConfig
	store       storage.Store
	logger      Logger
	taskService *TaskService
	taskChan    chan TaskContext
	executions  map[string]*executionState
	mu          sync.RWMutex
	wg          sync.WaitGroup
	ctx         context.Context
}

func NewWorkerPool(
	mainCtx context.Context,
	tasks map[string]ContextTaskFunc,
	taskDeps map[string][]string,
	store storage.Store,
	taskService *TaskService,
	logger Logger) *WorkerPool {
	return &WorkerPool{
		tasks:       tasks,
		taskDeps:    taskDeps,
		taskTypes:   make(map[string]string),
		taskConfigs: map[string]*models.TaskConfig{},
		store:       store,
		taskService: taskService,
		logger:      logger,
		executions:  make(map[string]*executionState),
		ctx:         mainCtx,
	}
}

// Start begins the worker pool with the specified number of workers
func (wp *WorkerPool) Start(workers int) {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	wp.taskChan = make(chan TaskContext, workers)
	for i := 0; i < workers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// Stop gracefully stops the worker pool
func (wp *WorkerPool) Stop() {
	// Close the task channel to stop accepting new tasks
	close(wp.taskChan)

	// Wait for all workers to finish
	wp.wg.Wait()

	// Clean up any remaining executions
	wp.mu.Lock()
	for execID := range wp.executions {
		wp.cleanupExecution(execID)
	}
	wp.mu.Unlock()
}

// UpdateTasks updates the tasks and dependencies
func (wp *WorkerPool) UpdateTasks(
	tasks map[string]ContextTaskFunc,
	taskDeps map[string][]string,
	taskTypes map[string]string,
	taskCfgs map[string]*models.TaskConfig,
) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.tasks = tasks
	wp.taskDeps = taskDeps
	wp.taskTypes = taskTypes
	wp.taskConfigs = taskCfgs
}

// handleContextCancellation handles the cancellation of tasks when either the task context or worker pool context is cancelled
func (wp *WorkerPool) handleContextCancellation(execID string, taskIDs []string, workflowCtx WorkflowContext, err error) {
	wp.logger.Infof("Context cancelled for execution %s: %v", execID, err)

	// Get task types first to avoid nested locks
	taskTypes := make(map[string]bool)
	wp.mu.RLock()
	for _, taskID := range taskIDs {
		taskTypes[taskID] = wp.taskTypes[taskID] == "task"
	}
	wp.mu.RUnlock()

	// Mark all pending tasks as failed and store error results
	for taskID, isTask := range taskTypes {
		if isTask {
			if updateErr := wp.taskService.UpdateTaskStatus(taskID, workflowCtx.WorkflowID, models.FailedTaskStatus, err.Error()); updateErr != nil {
				wp.logger.Errorf("Failed to update task %s status to FAILED: %v", taskID, updateErr)
			}
		}

		// Store error result so dependent tasks can access it
		workflowCtx.ResultsLock.Lock()
		workflowCtx.Results[taskID] = TaskResult(fmt.Sprintf("ERROR: %v", err))
		workflowCtx.ResultsLock.Unlock()
	}

	// Store the cancellation error in the state
	wp.mu.RLock()
	state, ok := wp.executions[execID]
	wp.mu.RUnlock()
	if ok {
		state.mu.Lock()
		state.taskErrors[execID] = err
		state.mu.Unlock()
	}

	// Clean up the execution - this will cause any remaining tasks in the queue to be skipped
	wp.cleanupExecution(execID)
}

// ExecuteTasks executes tasks for a specific workflow and execution id
func (wp *WorkerPool) ExecuteTasks(ctx context.Context, execID string, workflowCtx WorkflowContext, taskIDs []string) (map[string]TaskResult, map[string]error) {
	done := make(chan struct{})
	defer close(done)

	wp.mu.Lock()
	if _, exists := wp.executions[execID]; exists {
		wp.mu.Unlock()
		wp.logger.Errorf("execution %s already running", execID)
		return nil, map[string]error{"execId": fmt.Errorf("execution %s already running", execID)}
	}
	state := &executionState{
		taskErrors:   make(map[string]error),
		pendingCount: len(taskIDs),
		completeChan: make(chan struct{}),
		mu:           sync.RWMutex{},
	}
	wp.executions[execID] = state
	wp.mu.Unlock()

	// Monitor context cancellation
	go func() {
		select {
		case <-ctx.Done():
			wp.handleContextCancellation(execID, taskIDs, workflowCtx, ctx.Err())
		case <-wp.ctx.Done():
			wp.handleContextCancellation(execID, taskIDs, workflowCtx, wp.ctx.Err())
		case <-done:
			// exit goroutine
		}
	}()

	// queue tasks
	for _, taskID := range taskIDs {
		wp.mu.RLock()
		if _, exists := wp.tasks[taskID]; !exists {
			wp.mu.RUnlock()
			wp.cleanupExecution(execID)
			return nil, map[string]error{taskID: fmt.Errorf("dependency %s not registered", taskID)}
		}
		deps := wp.taskDeps[taskID]
		if deps == nil {
			deps = []string{}
		}
		isTask := wp.taskTypes[taskID] == "task"
		retries := 0
		var timeout *time.Duration
		taskCfg := wp.taskConfigs[taskID]
		// currently only supportive for tasks only
		if taskCfg != nil {
			retries = taskCfg.Retries
			timeout = taskCfg.Timeout
		}
		wp.mu.RUnlock()

		task := models.Task{
			ID:           taskID,
			WorkflowID:   workflowCtx.WorkflowID,
			Name:         taskID,
			Status:       models.PendingTaskStatus,
			ExecutionID:  execID,
			Dependencies: deps,
			Retries:      retries,
			Timeout:      timeout,
		}

		if isTask {
			if err := wp.taskService.SaveTask(task); err != nil {
				wp.logger.Errorf("Failed to save task %s: %v", taskID, err)
				wp.cleanupExecution(execID)
				return nil, map[string]error{taskID: err}
			}
		}

		// Queue task
		wp.taskChan <- TaskContext{Task: &task, WorkflowCtx: &workflowCtx, Ctx: ctx}
	}

	// Wait for completion
	<-state.completeChan

	state.mu.RLock()
	errors := make(map[string]error, len(state.taskErrors))
	for k, taskErr := range state.taskErrors {
		errors[k] = taskErr
	}
	state.mu.RUnlock()

	// Copy results with proper synchronization
	workflowCtx.ResultsLock.RLock()
	results := make(map[string]TaskResult, len(workflowCtx.Results))
	for k, taskResult := range workflowCtx.Results {
		results[k] = taskResult
	}
	workflowCtx.ResultsLock.RUnlock()

	return results, errors
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	for taskCtx := range wp.taskChan {
		if wp.ctx.Err() != nil {
			return
		}

		// Check if the execution still exists before processing the task
		wp.mu.RLock()
		_, executionExists := wp.executions[taskCtx.Task.ExecutionID]
		wp.mu.RUnlock()

		if !executionExists {
			// Execution has been cleaned up, mark task as failed and skip processing
			wp.logger.Infof("Skipping task %s: execution %s has been cleaned up", taskCtx.Task.ID, taskCtx.Task.ExecutionID)
			wp.markTaskFailed(*taskCtx.Task, fmt.Errorf("execution %s has been cleaned up", taskCtx.Task.ExecutionID), taskCtx.WorkflowCtx)
			continue
		}

		wp.executeTask(taskCtx)
	}
}

func (wp *WorkerPool) canRunTask(task models.Task) (bool, error) {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	if _, ok := wp.executions[task.ExecutionID]; !ok {
		wp.logger.Errorf("Error trying to run task %s: Execution ID: %s not found", task.ID, task.ExecutionID)
		return false, fmt.Errorf("execution %s not found", task.ExecutionID)
	}
	return wp.taskService.CanRunTask(task)
}

func (wp *WorkerPool) executeTask(taskCtx TaskContext) {
	// check if task can run
	task := *taskCtx.Task
	workflowCtx := *taskCtx.WorkflowCtx
	ctx := taskCtx.Ctx
	canRun, err := wp.canRunTask(task)
	if err != nil {
		wp.markTaskFailed(*taskCtx.Task, err, &workflowCtx)
		return
	}
	if !canRun {
		wp.taskChan <- taskCtx
		// wp.logger.Infof("Requeued task %s due to unready dependencies", taskCtx.Task.ID)
		return
	}

	// check execution
	wp.mu.RLock()
	state, ok := wp.executions[taskCtx.Task.ExecutionID]
	wp.mu.RUnlock()
	if !ok {
		wp.logger.Errorf("Error executing task with id %s: Unknown execution with id: %s", task.ID, task.ExecutionID)
		// Mark task as failed since execution was cleaned up (likely due to context cancellation)
		wp.markTaskFailed(task, fmt.Errorf("execution %s not found", task.ExecutionID), &workflowCtx)
		return
	}

	// get task function
	wp.mu.RLock()
	taskFn, ok := wp.tasks[task.ID]
	wp.mu.RUnlock()
	if !ok {
		err := fmt.Errorf("task function %s not found", task.ID)
		wp.logger.Errorf("Error retrieving task function: %v", err)
		wp.markTaskFailed(task, err, &workflowCtx)
		return
	}

	args := make([]TaskResult, len(task.Dependencies))
	for i, dependency := range task.Dependencies {
		workflowCtx.ResultsLock.RLock()
		res, ok := workflowCtx.Results[dependency]
		workflowCtx.ResultsLock.RUnlock()
		if !ok {
			// Dependency result not found, requeue the task and try again later
			wp.logger.Infof("Dependency %s result not found for task %s, requeuing", dependency, task.ID)
			wp.taskChan <- taskCtx
			return
		}
		args[i] = res
	}

	// Check if this is a task (not a flow) to update status
	wp.mu.RLock()
	isTask := wp.taskTypes[task.ID] == "task"
	wp.mu.RUnlock()
	// Execute task with retries
	if isTask {
		if updateErr := wp.taskService.UpdateTaskStatus(task.ID, task.WorkflowID, models.RunningTaskStatus, ""); updateErr != nil {
			wp.logger.Errorf("Failed to update task %s status to RUNNING: %v", task.ID, updateErr)
			return
		}
	}

	// execute task
	var result TaskResult
	var taskErr error

	timeout := task.Timeout
	if timeout == nil {
		// timeout is not defined on task, going with default 1min timeout
		var defaultTimeout = DefaultTaskTimeout
		timeout = &defaultTimeout
	}

	resultCh := make(chan struct {
		res TaskResult
		err error
	}, 1)

	// Create a context that is cancelled when either the task context or worker pool context is cancelled
	execCtx, cancel := context.WithCancel(ctx)
	// Monitor both contexts
	go func() {
		select {
		case <-execCtx.Done():
			wp.logger.Infof("Task %s execution context done", task.ID)
			cancel()
		case <-wp.ctx.Done():
			wp.logger.Infof("Worker pool context done for task %s", task.ID)
			cancel() // Cancel the execution context
		}
	}()

	// Function to update task status
	updateTaskStatus := func(status models.TaskStatus, errMsg string) {
		if isTask {
			if updateErr := wp.taskService.UpdateTaskStatus(task.ID, task.WorkflowID, status, errMsg); updateErr != nil {
				wp.logger.Errorf("Failed to update task %s status to %s: %v", task.ID, status, updateErr)
			}
		}
	}

	// Function to update task attempts
	updateTaskAttempts := func(attempts int) {
		if isTask {
			if updateErr := wp.taskService.UpdateTaskAttempts(task.ID, task.WorkflowID, attempts); updateErr != nil {
				wp.logger.Errorf("Failed to update task %s attempts to %d: %v", task.ID, attempts, updateErr)
			}
		}
	}

	for attempt := 0; attempt <= task.Retries; attempt++ {
		wp.logger.Infof("Starting task %s attempt %d", task.ID, attempt+1)

		// Create a timeout context for this attempt
		timeoutCtx, timeoutCancel := context.WithTimeout(execCtx, *timeout)

		// Execute the task in a goroutine
		go func() {
			defer timeoutCancel()
			updateTaskAttempts(attempt + 1)
			res, err := taskFn(timeoutCtx, args...)
			resultCh <- struct {
				res TaskResult
				err error
			}{res, err}
		}()

		// Wait for either the result or context cancellation
		select {
		case r := <-resultCh:
			result, taskErr = r.res, r.err
			wp.logger.Infof("Task %s received result: %v, error: %v", task.ID, result, taskErr)
			if taskErr == nil {
				break
			}
			task.ErrorMsg = taskErr.Error()
			if attempt < task.Retries {
				wp.logger.Infof("Retrying task %s (attempt %d/%d): %v", task.ID, attempt+1, task.Retries, taskErr)
				<-time.After(100 * time.Millisecond)
				continue
			}
		case <-timeoutCtx.Done():
			taskErr = timeoutCtx.Err()
			wp.logger.Infof("Task %s timeout reached: %v", task.ID, taskErr)
			updateTaskStatus(models.FailedTaskStatus, taskErr.Error())
			if attempt < task.Retries {
				wp.logger.Infof("Retrying task %s due to timeout (attempt %d/%d)", task.ID, attempt+1, task.Retries)
				<-time.After(100 * time.Millisecond)
				continue
			}
		}
		break
	}

	if taskErr != nil {
		wp.logger.Infof("Task %s failed after %d retries: %v", task.ID, task.Retries, taskErr)
		task.ErrorMsg = taskErr.Error()
		state.mu.Lock()
		state.taskErrors[task.ID] = taskErr
		state.mu.Unlock()
		updateTaskStatus(models.FailedTaskStatus, taskErr.Error())

		// Store error result so dependent tasks can access it
		workflowCtx.ResultsLock.Lock()
		workflowCtx.Results[task.ID] = TaskResult(fmt.Sprintf("ERROR: %v", taskErr))
		workflowCtx.ResultsLock.Unlock()
	} else {
		wp.logger.Infof("Task %s completed successfully", task.ID)
		workflowCtx.ResultsLock.Lock()
		workflowCtx.Results[task.ID] = result
		workflowCtx.ResultsLock.Unlock()

		updateTaskStatus(models.CompletedTaskStatus, "")

		if isTask {
			task.Status = models.CompletedTaskStatus
			finishedAt := time.Now()
			task.FinishedAt = &finishedAt
			if saveErr := wp.taskService.SaveTask(task); saveErr != nil {
				wp.logger.Errorf("Failed to save task %s with FinishedAt: %v", task.ID, saveErr)
				return
			}
		}
	}

	wp.logger.Infof("Task %s cleanup starting", task.ID)
	state.mu.Lock()
	state.pendingCount--
	if state.pendingCount == 0 {
		wp.logger.Infof("Task %s is last pending task, cleaning up execution", task.ID)
		wp.cleanupExecution(task.ExecutionID)
	}
	state.mu.Unlock()
	wp.logger.Infof("Task %s cleanup completed", task.ID)

	// Cancel the context only after we're completely done with the task
	cancel()
	wp.logger.Infof("Task %s execution context cancelled", task.ID)
}

func (wp *WorkerPool) markTaskFailed(task models.Task, err error, workflowCtx *WorkflowContext) {
	wp.mu.RLock()
	state, ok := wp.executions[task.ExecutionID]
	wp.mu.RUnlock()
	if !ok {
		wp.logger.Errorf("Cannot mark task %s as failed: execution %s not found", task.ID, task.ExecutionID)
		// Even if execution is not found, we should still update the task status and store the error result
		// Check if this is a task (not a flow) to update status
		wp.mu.RLock()
		isTask := wp.taskTypes[task.ID] == "task"
		wp.mu.RUnlock()

		if isTask {
			if updateErr := wp.taskService.UpdateTaskStatus(task.ID, task.WorkflowID, models.FailedTaskStatus, err.Error()); updateErr != nil {
				wp.logger.Errorf("Failed to update task %s status to FAILED: %v", task.ID, updateErr)
			}
		}

		// Store error result so dependent tasks can access it
		workflowCtx.ResultsLock.Lock()
		workflowCtx.Results[task.ID] = TaskResult(fmt.Sprintf("ERROR: %v", err))
		workflowCtx.ResultsLock.Unlock()
		return
	}

	// Check if this is a task (not a flow) to update status
	wp.mu.RLock()
	isTask := wp.taskTypes[task.ID] == "task"
	wp.mu.RUnlock()

	state.mu.Lock()
	state.taskErrors[task.ID] = err
	state.mu.Unlock()

	if isTask {
		if updateErr := wp.taskService.UpdateTaskStatus(task.ID, task.WorkflowID, models.FailedTaskStatus, err.Error()); updateErr != nil {
			wp.logger.Errorf("Failed to update task %s status to FAILED: %v", task.ID, updateErr)
		}
	}

	// Store error result so dependent tasks can access it
	workflowCtx.ResultsLock.Lock()
	workflowCtx.Results[task.ID] = TaskResult(fmt.Sprintf("ERROR: %v", err))
	workflowCtx.ResultsLock.Unlock()

	state.mu.Lock()
	state.pendingCount--
	if state.pendingCount == 0 {
		wp.cleanupExecution(task.ExecutionID)
	}
	state.mu.Unlock()
}

func (wp *WorkerPool) cleanupExecution(execID string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if state, ok := wp.executions[execID]; ok {
		state.cleanupOnce.Do(func() {
			close(state.completeChan)
			delete(wp.executions, execID)
			wp.logger.Infof("Cleaned up execution: %s", execID)
		})
	}
}
