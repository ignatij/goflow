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
	taskStatus   map[string]string
	taskResults  map[string]TaskResult
	taskErrors   map[string]error
	taskCount    int           // Total tasks in execution
	pendingCount int           // Tasks not yet completed or failed
	completeChan chan struct{} // Signals completion or error
}

// WorkerPool manages parallel task execution with dependency enforcement
type WorkerPool struct {
	tasks      map[string]TaskFunc
	taskDeps   map[string][]string
	store      storage.Store
	logger     Logger
	taskChan   chan models.Task
	executions map[string]*executionState
	mu         sync.RWMutex
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewWorkerPool(
	tasks map[string]TaskFunc,
	taskDeps map[string][]string,
	store storage.Store,
	logger Logger) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		tasks:      tasks,
		taskDeps:   taskDeps,
		store:      store,
		logger:     logger,
		taskChan:   make(chan models.Task, 100),
		executions: make(map[string]*executionState),
		ctx:        ctx,
		cancel:     cancel,
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
func (wp *WorkerPool) UpdateTasks(tasks map[string]TaskFunc, taskDeps map[string][]string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.tasks = tasks
	wp.taskDeps = taskDeps
}

// ExecuteTasks executes tasks for a specific workflow and flow
func (wp *WorkerPool) ExecuteTasks(execID string, wfID int64, taskIDs []string) (map[string]TaskResult, map[string]error) {
	wp.mu.Lock()
	if _, exists := wp.executions[execID]; exists {
		wp.mu.Unlock()
		wp.logger.Errorf("execution %s already running", execID)
		return nil, map[string]error{"execId": fmt.Errorf("execution %s already running", execID)}
	}
	state := &executionState{
		taskStatus:   make(map[string]string),
		taskResults:  make(map[string]TaskResult),
		taskErrors:   make(map[string]error),
		taskCount:    len(taskIDs),
		pendingCount: len(taskIDs),
		completeChan: make(chan struct{}),
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

		// Save task to store
		task := models.Task{
			ID:           taskID,
			WorkflowID:   wfID,
			Name:         taskID,
			Status:       "PENDING",
			ExecutionID:  execID,
			Dependencies: deps,
			Retries:      2,
		}

		if err := wp.store.SaveTask(task); err != nil {
			wp.logger.Errorf("Failed to save task %s: %v", taskID, err)
			wp.cleanupExecution(execID)
			return nil, map[string]error{taskID: err}
		}

		state.taskStatus[taskID] = "PENDING"

		// Queue task
		select {
		case wp.taskChan <- task:
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

	results := make(map[string]TaskResult, len(state.taskResults))
	for k, taskResult := range state.taskResults {
		results[k] = taskResult
	}

	errors := make(map[string]error, len(state.taskErrors))
	for k, taskErr := range state.taskErrors {
		errors[k] = taskErr
	}

	wp.cleanupExecution(execID)
	return results, errors
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	for task := range wp.taskChan {
		if wp.ctx.Err() != nil {
			return
		}
		if wp.canRunTask(task) {
			wp.executeTask(task)
		} else {
			select {
			case wp.taskChan <- task:
			case <-wp.ctx.Done():
			}
		}

	}
}

func (wp *WorkerPool) canRunTask(task models.Task) bool {
	wp.mu.RLock()
	if _, ok := wp.executions[task.ExecutionID]; !ok {
		wp.logger.Errorf("Error trying to run task %s: Execution ID: %s not found", task.ID, task.ExecutionID)
		wp.mu.RUnlock()
		return false
	}
	wp.mu.RUnlock()
	for _, dep := range task.Dependencies {
		d, err := wp.store.GetTask(dep, task.WorkflowID)
		if err != nil {
			wp.logger.Errorf("Error retrieving dependency %s: %v", dep, err)
			return false
		}
		if d.Status != "COMPLETED" {
			wp.logger.Infof("Cannot run task %s as dependency %s is not in status COMPLETED", task.ID, dep)
			return false
		}
	}
	return true
}

func (wp *WorkerPool) executeTask(task models.Task) {
	latestTask, err := wp.store.GetTask(task.ID, task.WorkflowID)
	if err != nil {
		wp.logger.Errorf("Error retrieving task with id %s: %v", task.ID, err)
		return
	}

	if latestTask.Status != "PENDING" {
		wp.logger.Infof("Task status of task with id: %s isn't pending. Skipping...", task.ID)
		return
	}

	// Check execution
	wp.mu.RLock()
	state, ok := wp.executions[task.ExecutionID]
	if !ok {
		wp.mu.RUnlock()
		wp.logger.Errorf("Error executing task with id %s: Unknown execution with id: %s", task.ID, task.ExecutionID)
		return
	}
	wp.mu.RUnlock()

	// Mark as running
	task.Status = "RUNNING"
	startedAt := time.Now()
	task.StartedAt = &startedAt
	if err := wp.store.SaveTask(task); err != nil {
		wp.logger.Errorf("Error executing task with id %s: Cannot update task status to 'RUNNING'", task.ID)
		return
	}

	// Get task function
	wp.mu.RLock()
	taskFn, ok := wp.tasks[task.ID]
	wp.mu.RUnlock()
	if !ok {
		err := fmt.Errorf("task function %s not found", task.ID)
		wp.logger.Errorf("Error retrieving task function: %v", err)
		wp.markTaskFailed(task, state, err)
		return
	}

	// Collect dependency results
	args := make([]TaskResult, len(task.Dependencies))
	for i, dependency := range task.Dependencies {
		res, ok := state.taskResults[dependency]
		if !ok {
			err := fmt.Errorf("result for dependency %s not found", dependency)
			wp.logger.Errorf("Error retrieving dependency result: %v", err)
			wp.markTaskFailed(task, state, err)
			return
		}
		args[i] = res
	}

	// Execute task with retries
	var result TaskResult
	for attempt := 0; attempt < task.Retries; attempt++ {
		result, err = taskFn(args...)
		if err == nil {
			break
		}
		task.Attempts++
		task.ErrorMsg = err.Error()
		if attempt < task.Retries {
			wp.logger.Infof("Retrying task %s (attempt %d/%d): %v", task.ID, attempt+1, task.Retries, err)
			time.Sleep(100 * time.Millisecond) // backoff
			continue
		}
	}

	finishedAt := time.Now()
	task.FinishedAt = &finishedAt

	// Update task state
	if err != nil {
		task.Status = "FAILED"
		task.ErrorMsg = err.Error()
		state.taskErrors[task.ID] = err
	} else {
		task.Status = "COMPLETED"
		state.taskResults[task.ID] = result
	}

	state.taskStatus[task.ID] = task.Status
	state.pendingCount--
	if state.pendingCount == 0 || len(state.taskErrors) > 0 {
		select {
		case state.completeChan <- struct{}{}:
		default:
		}
	}

	if err := wp.store.SaveTask(task); err != nil {
		wp.logger.Errorf("Failed to save task %s: %v", task.ID, err)
		return
	}
}

func (wp *WorkerPool) markTaskFailed(task models.Task, state *executionState, err error) {
	task.Status = "FAILED"
	task.ErrorMsg = err.Error()
	if saveErr := wp.store.SaveTask(task); saveErr != nil {
		wp.logger.Errorf("Failed to save task %s: %v", task.ID, saveErr)
	}

	state.taskStatus[task.ID] = "FAILED"
	state.taskErrors[task.ID] = err
	state.pendingCount--
	if state.pendingCount == 0 || len(state.taskErrors) > 0 {
		select {
		case state.completeChan <- struct{}{}:
		default:
		}
	}

}

func (wp *WorkerPool) cleanupExecution(execID string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if state, ok := wp.executions[execID]; ok {
		close(state.completeChan)
		delete(wp.executions, execID)
	}
}
