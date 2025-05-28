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

// executionState holds state for a single execution (workflow + flow)
type executionState struct {
	taskErrors   map[string]error
	pendingCount int           // Tasks not yet completed or failed
	completeChan chan struct{} // Signals completion or error
	mu           sync.RWMutex
}

type WorkflowContext struct {
	WorkflowID  int64
	Results     map[string]TaskResult // shared across all flows in workflow
	ResultsLock *sync.RWMutex         // protect access to shared results
}

type TaskContext struct {
	Task *models.Task
	Ctx  *WorkflowContext
}

// WorkerPool manages parallel task execution with dependency enforcement
type WorkerPool struct {
	tasks       map[string]TaskFunc
	taskDeps    map[string][]string
	taskTypes   map[string]string // can be "task" or "flow"
	store       storage.Store
	logger      Logger
	taskService *TaskService
	taskChan    chan TaskContext
	executions  map[string]*executionState
	mu          sync.RWMutex
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewWorkerPool(
	tasks map[string]TaskFunc,
	taskDeps map[string][]string,
	store storage.Store,
	taskService *TaskService,
	logger Logger) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		tasks:       tasks,
		taskDeps:    taskDeps,
		taskTypes:   make(map[string]string),
		store:       store,
		taskService: taskService,
		logger:      logger,
		taskChan:    make(chan TaskContext, 100),
		executions:  make(map[string]*executionState),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins the worker pool with the specified number of workers
func (wp *WorkerPool) Start(workers int) {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	for i := 0; i < workers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// UpdateTasks updates the tasks and dependencies
func (wp *WorkerPool) UpdateTasks(tasks map[string]TaskFunc, taskDeps map[string][]string, taskTypes map[string]string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.tasks = tasks
	wp.taskDeps = taskDeps
	wp.taskTypes = taskTypes
}

// ExecuteTasks executes tasks for a specific workflow and execution id
func (wp *WorkerPool) ExecuteTasks(execID string, ctx WorkflowContext, taskIDs []string) (map[string]TaskResult, map[string]error) {
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
		wp.mu.RUnlock()

		task := models.Task{
			ID:           taskID,
			WorkflowID:   ctx.WorkflowID,
			Name:         taskID,
			Status:       "PENDING",
			ExecutionID:  execID,
			Dependencies: deps,
			Retries:      2,
		}

		if isTask {
			if err := wp.taskService.SaveTask(task); err != nil {
				wp.logger.Errorf("Failed to save task %s: %v", taskID, err)
				wp.cleanupExecution(execID)
				return nil, map[string]error{taskID: err}
			}
		}

		// Queue task
		select {
		case wp.taskChan <- TaskContext{Task: &task, Ctx: &ctx}:
		case <-wp.ctx.Done():
			wp.cleanupExecution(execID)
			return nil, map[string]error{execID: wp.ctx.Err()}
		}

	}

	// Wait for completion
	select {
	case <-state.completeChan:
	// default timeout of 60 minutes, should rethink this
	case <-time.After(60 * time.Minute):
		wp.cleanupExecution(execID)
		return nil, map[string]error{execID: fmt.Errorf("execution timed out")}
	}
	state.mu.RLock()
	results := make(map[string]TaskResult, len(ctx.Results))
	for k, taskResult := range ctx.Results {
		results[k] = taskResult
	}

	errors := make(map[string]error, len(state.taskErrors))
	for k, taskErr := range state.taskErrors {
		errors[k] = taskErr
	}
	state.mu.RUnlock()

	wp.cleanupExecution(execID)
	return results, errors
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	for taskCtx := range wp.taskChan {
		if wp.ctx.Err() != nil {
			return
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
	ctx := *taskCtx.Ctx
	canRun, err := wp.canRunTask(*taskCtx.Task)
	if err != nil {
		wp.markTaskFailed(*taskCtx.Task, err)
		return
	}
	if !canRun {
		select {
		case wp.taskChan <- taskCtx:
			wp.logger.Infof("Requeued task %s due to unready dependencies", taskCtx.Task.ID)
		case <-wp.ctx.Done():
			wp.logger.Infof("Skipped requeue of task %s due to context cancellation", taskCtx.Task.ID)
		}
		return
	}

	// check execution
	wp.mu.RLock()
	state, ok := wp.executions[taskCtx.Task.ExecutionID]
	wp.mu.RUnlock()
	if !ok {
		wp.logger.Errorf("Error executing task with id %s: Unknown execution with id: %s", task.ID, task.ExecutionID)
		return
	}

	// get task function
	wp.mu.RLock()
	taskFn, ok := wp.tasks[task.ID]
	wp.mu.RUnlock()
	if !ok {
		err := fmt.Errorf("task function %s not found", task.ID)
		wp.logger.Errorf("Error retrieving task function: %v", err)
		wp.markTaskFailed(task, err)
		return
	}

	args := make([]TaskResult, len(task.Dependencies))
	for i, dependency := range task.Dependencies {
		ctx.ResultsLock.RLock()
		res, ok := ctx.Results[dependency]
		ctx.ResultsLock.RUnlock()
		if !ok {
			err := fmt.Errorf("result for dependency %s not found", dependency)
			wp.logger.Errorf("Error retrieving dependency result: %v", err)
			wp.markTaskFailed(task, err)
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

	var result TaskResult
	var taskErr error
	for attempt := 0; attempt <= task.Retries; attempt++ {
		result, taskErr = taskFn(args...)
		if taskErr == nil {
			break
		}
		task.Attempts++
		task.ErrorMsg = taskErr.Error()
		if attempt < task.Retries {
			wp.logger.Infof("Retrying task %s (attempt %d/%d): %v", task.ID, attempt+1, task.Retries, taskErr)
			time.Sleep(100 * time.Millisecond) // backoff
			continue
		}
	}

	state.mu.Lock()
	if taskErr != nil {
		task.ErrorMsg = taskErr.Error()
		state.taskErrors[task.ID] = taskErr
		if isTask {
			if updateErr := wp.taskService.UpdateTaskStatus(task.ID, task.WorkflowID, models.FailedTaskStatus, taskErr.Error()); updateErr != nil {
				wp.logger.Errorf("Failed to update task %s status to FAILED: %v", task.ID, updateErr)
				return
			}
		}
	} else {
		// state.taskResults[task.ID] = result
		ctx.ResultsLock.Lock()
		ctx.Results[task.ID] = result
		ctx.ResultsLock.Unlock()

		if isTask {
			if updateErr := wp.taskService.UpdateTaskStatus(task.ID, task.WorkflowID, models.CompletedTaskStatus, ""); updateErr != nil {
				wp.logger.Errorf("Failed to update task %s status to COMPLETED: %v", task.ID, updateErr)
				return
			}

			task.Status = "COMPLETED"
			finishedAt := time.Now()
			task.FinishedAt = &finishedAt
			if saveErr := wp.taskService.SaveTask(task); saveErr != nil {
				wp.logger.Errorf("Failed to save task %s with FinishedAt: %v", task.ID, saveErr)
				return
			}
		}
	}
	state.pendingCount--
	if state.pendingCount == 0 || len(state.taskErrors) > 0 {
		select {
		case state.completeChan <- struct{}{}:
		default:
		}
	}
	state.mu.Unlock()
}

func (wp *WorkerPool) markTaskFailed(task models.Task, err error) {
	wp.mu.RLock()
	state, ok := wp.executions[task.ExecutionID]
	wp.mu.RUnlock()
	if !ok {
		wp.logger.Errorf("Cannot mark task %s as failed: execution %s not found", task.ID, task.ExecutionID)
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
		if updateErr := wp.taskService.UpdateTaskStatus(task.ID, task.WorkflowID, "FAILED", err.Error()); updateErr != nil {
			wp.logger.Errorf("Failed to update task %s status to FAILED: %v", task.ID, updateErr)
		}
	}

	state.mu.Lock()
	state.pendingCount--
	if state.pendingCount == 0 || len(state.taskErrors) > 0 {
		select {
		case state.completeChan <- struct{}{}:
		default:
		}
	}
	state.mu.Unlock()
}

func (wp *WorkerPool) cleanupExecution(execID string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if state, ok := wp.executions[execID]; ok {
		close(state.completeChan)
		delete(wp.executions, execID)
	}
}
