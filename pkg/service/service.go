package service

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/pkg/errors"
)

// Logger defines the logging interface for WorkflowService
type Logger interface {
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// TaskResult represents the output of a task or flow
type TaskResult interface{}

// TaskFunc is a function defining task/flow, with dependencies as parameters
type TaskFunc func(args ...TaskResult) (TaskResult, error)
type ContextTaskFunc func(ctx context.Context, args ...TaskResult) (TaskResult, error)

func WrapTaskFunc(fn TaskFunc) ContextTaskFunc {
	return func(ctx context.Context, args ...TaskResult) (TaskResult, error) {
		return fn(args...)
	}
}

// WorkflowService manages workflow instances and their flow executions.
// A workflow is a specific instance of a pipeline run, persisted with a unique ID.
// A flow is a reusable definition of tasks and dependencies, executed within a workflow.
type WorkflowService struct {
	store       storage.Store
	ctx         context.Context
	logger      Logger
	tasks       map[string]ContextTaskFunc
	taskDeps    map[string][]string
	taskConfigs map[string]*models.TaskConfig
	flows       map[string]ContextTaskFunc
	flowDeps    map[string][]string
	results     map[int64]map[string]TaskResult
	wp          *WorkerPool
	mu          sync.RWMutex
}

func NewWorkflowService(ctx context.Context, store storage.Store, logger Logger) *WorkflowService {
	taskService := NewTaskService(store, logger)
	wp := NewWorkerPool(ctx, make(map[string]ContextTaskFunc), make(map[string][]string), store, taskService, logger)
	wp.Start(0)
	return &WorkflowService{
		store:       store,
		ctx:         ctx,
		logger:      logger,
		tasks:       make(map[string]ContextTaskFunc),
		taskDeps:    make(map[string][]string),
		taskConfigs: make(map[string]*models.TaskConfig),
		flows:       make(map[string]ContextTaskFunc),
		flowDeps:    make(map[string][]string),
		results:     make(map[int64]map[string]TaskResult),
		wp:          wp,
	}
}

// RegisterTask registers a task, inferring dependencies from parameter names
func (s *WorkflowService) RegisterTask(name string, fn ContextTaskFunc, deps []string, opts ...models.TaskOption) error {
	if err := validateTaskFunc(fn); err != nil {
		return fmt.Errorf("invalid task function for '%s': %v", name, err)
	}
	if len(name) == 0 {
		return errors.New("empty task name")
	}

	cfg := &models.TaskConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	s.mu.Lock()
	s.tasks[name] = fn
	s.taskDeps[name] = deps
	s.taskConfigs[name] = cfg
	// Update WorkerPool
	combinedTasks := make(map[string]ContextTaskFunc, len(s.tasks))
	combinedDeps := make(map[string][]string, len(s.taskDeps))
	combinedTypes := make(map[string]string, len(s.wp.taskTypes))
	combinedCfgs := make(map[string]*models.TaskConfig, len(s.taskConfigs))
	for k, v := range s.tasks {
		combinedTasks[k] = v
		combinedDeps[k] = s.taskDeps[k]
		combinedTypes[k] = "task"
	}
	for k, v := range s.flows {
		combinedTasks[k] = v
		combinedDeps[k] = s.flowDeps[k]
		combinedTypes[k] = "flow"
	}
	for k, v := range s.taskConfigs {
		combinedCfgs[k] = v
	}
	s.mu.Unlock()
	s.wp.UpdateTasks(combinedTasks, combinedDeps, combinedTypes, combinedCfgs)
	s.logger.Infof("Registered task '%s' with dependencies '%v'", name, deps)
	return nil
}

// RegisterFlow registers a flow, inferring dependencies from parameter names
func (s *WorkflowService) RegisterFlow(name string, fn ContextTaskFunc, deps []string) error {
	if len(name) == 0 {
		return errors.New("empty flow name")
	}
	if err := validateTaskFunc(fn); err != nil {
		return fmt.Errorf("invalid flow function for '%s': %v", name, err)
	}
	s.mu.Lock()
	s.flows[name] = fn
	s.flowDeps[name] = deps
	// Update WorkerPool
	combinedTasks := make(map[string]ContextTaskFunc)
	combinedDeps := make(map[string][]string)
	combinedTypes := make(map[string]string)

	for k, v := range s.tasks {
		combinedTasks[k] = v
		combinedDeps[k] = s.taskDeps[k]
		combinedTypes[k] = "task"
	}
	for k, v := range s.flows {
		combinedTasks[k] = v
		combinedDeps[k] = s.flowDeps[k]
		combinedTypes[k] = "flow"
	}
	taskCfgs := s.taskConfigs
	s.wp.UpdateTasks(combinedTasks, combinedDeps, combinedTypes, taskCfgs)
	s.mu.Unlock()
	s.logger.Infof("Registered flow '%s' with dependencies %v", name, deps)
	return nil
}

// validateTaskFunc checks if the function matches the expected signature
func validateTaskFunc(fn ContextTaskFunc) error {
	fnType := reflect.TypeOf(fn)
	if fn == nil || fnType.Kind() != reflect.Func {
		return errors.New("must be a function")
	}
	// Ensure 2 returns: first is any type (TaskResult), second is error
	if fnType.NumOut() != 2 || fnType.Out(0).Kind() == reflect.Invalid || fnType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		return errors.New("must return (TaskResult, error)")
	}
	// Note: fnType.Out(0) isn't checked against TaskResult (interface{}) explicitly,
	// as any type satisfies it. This allows flexibility (e.g., string, int).
	return nil
}

// ExecuteFlow runs a flow for a given workflow
func (s *WorkflowService) ExecuteFlow(ctx context.Context, workflowID int64, flowName string) (TaskResult, error) {
	txStore, err := s.store.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if commitErr := txStore.Commit(); commitErr != nil {
			s.logger.Errorf("Failed to commit: %v", commitErr)
			err = commitErr
		}
	}()

	// Verify workflow exists
	_, err = txStore.GetWorkflow(workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow %d not found: %v", workflowID, err)
	}

	s.mu.RLock()
	if _, ok := s.flows[flowName]; !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("flow '%s' is not registered", flowName)
	}
	s.mu.RUnlock()

	// Set workflow to RUNNING
	if err := txStore.UpdateWorkflowStatus(workflowID, models.RunningWorkflowStatus); err != nil {
		return nil, fmt.Errorf("failed to set workflow %d to RUNNING: %v", workflowID, err)
	}

	// Compute execution order
	order, err := s.topologicalSort(flowName)
	if err != nil {
		errU := txStore.UpdateWorkflowStatus(workflowID, models.FailedWorkflowStatus)
		if errU != nil {
			return nil, errors.Wrap(errors.WithMessage(err, fmt.Sprintf("failed to update workflow status: %v", errU)), "topological sort failed")
		}
		return nil, err
	}

	// Initialize results map for this workflow
	s.mu.Lock()
	if _, ok := s.results[workflowID]; !ok {
		s.results[workflowID] = make(map[string]TaskResult)
	}
	s.mu.Unlock()

	// Execute tasks and flows using WorkerPool
	execID := fmt.Sprintf("exec-%d-%s", workflowID, flowName)
	workflowCtx := WorkflowContext{
		WorkflowID:  workflowID,
		Results:     s.results[workflowID],
		ResultsLock: &s.mu, // Use the service's mutex to protect the shared results map
	}
	_, errs := s.wp.ExecuteTasks(ctx, execID, workflowCtx, order)

	if len(errs) > 0 {
		if errU := txStore.UpdateWorkflowStatus(workflowID, models.FailedWorkflowStatus); errU != nil {
			return nil, errors.Wrap(errU, "failed to update workflow status after errors")
		}
		var combinedErrs []string
		for id, err := range errs {
			combinedErrs = append(combinedErrs, fmt.Sprintf("%s: %v", id, err))
		}
		return nil, fmt.Errorf("execution of flow '%s' failed: %s", flowName, strings.Join(combinedErrs, "; "))
	}

	// Mark workflow as completed
	if err := txStore.UpdateWorkflowStatus(workflowID, models.CompletedWorkflowStatus); err != nil {
		return nil, fmt.Errorf("failed to set workflow %d to COMPLETED: %v", workflowID, err)
	}

	// Return result
	s.mu.RLock()
	result, ok := s.results[workflowID][flowName]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no result for flow '%s'", flowName)
	}
	s.logger.Infof("Executed flow '%s' for workflow %d", flowName, workflowID)
	return result, nil
}

// topologicalSort computes the execution order for a flow
func (s *WorkflowService) topologicalSort(flowName string) ([]string, error) {
	graph := make(map[string][]string)
	inDegree := make(map[string]int)
	allNodes := make(map[string]struct{})

	// build graph
	s.mu.RLock()
	for name, deps := range s.taskDeps {
		graph[name] = deps
		allNodes[name] = struct{}{}
		inDegree[name] = 0
	}
	for name, deps := range s.flowDeps {
		graph[name] = deps
		allNodes[name] = struct{}{}
		inDegree[name] = 0
	}
	s.mu.RUnlock()

	// Calculate in-degrees
	for node, deps := range graph {
		for _, dep := range deps {
			if _, ok := allNodes[dep]; !ok {
				return nil, fmt.Errorf("dependency '%s' for '%s' not registered", dep, node)
			}
			inDegree[dep]++
		}
	}

	// Find nodes with no dependencies
	var queue []string
	for node := range allNodes {
		if inDegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	// Topological sort
	var sorted []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		sorted = append(sorted, curr)

		for _, dep := range graph[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	if len(sorted) != len(allNodes) {
		return nil, errors.New("cycle detected in dependencies")
	}

	// Filter to reachable nodes from flowName
	reachable := make(map[string]struct{})
	var visit func(string)
	visit = func(name string) {
		reachable[name] = struct{}{}
		for _, dep := range graph[name] {
			if _, ok := reachable[dep]; !ok {
				visit(dep)
			}
		}
	}
	visit(flowName)

	var flowOrder []string
	for _, node := range sorted {
		if _, ok := reachable[node]; ok {
			flowOrder = append(flowOrder, node)
		}
	}
	slices.Reverse(flowOrder)
	return flowOrder, nil
}

func (s *WorkflowService) CreateWorkflow(name string) (id int64, err error) {
	if name == "" {
		return 0, errors.New("workflow name cannot be empty")
	}
	if len(name) > 100 {
		return 0, errors.New("workflow name too long (max 100 characters)")
	}
	txStore, err := s.store.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			if rollbackErr := txStore.Rollback(); rollbackErr != nil {
				s.logger.Errorf("Failed to rollback after error: %v (original error: %v)", rollbackErr, err)
			}
			return
		}
		if commitErr := txStore.Commit(); commitErr != nil {
			s.logger.Errorf("Failed to commit: %v", commitErr)
			err = commitErr
		}
	}()

	wf := models.Workflow{
		Name:      name,
		Status:    models.PendingWorkflowStatus,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	id, err = txStore.SaveWorkflow(wf)
	if err != nil {
		return 0, err
	}
	s.logger.Infof("Created workflow '%s' with ID %d", name, id)
	return id, nil
}

// UpdateWorkflowStatus updates the status of an existing workflow by ID.
func (s *WorkflowService) UpdateWorkflowStatus(id int64, status string) error {
	// Validate inputs
	if id <= 0 {
		return errors.New("workflow ID must be positive")
	}
	wfStatus := models.WorkflowStatus(status)
	switch wfStatus {
	case models.PendingWorkflowStatus, models.RunningWorkflowStatus,
		models.CompletedWorkflowStatus, models.FailedWorkflowStatus:
		// Valid status, proceed
	default:
		return errors.New("invalid status; must be 'PENDING', 'RUNNING', 'COMPLETED', or 'FAILED'")
	}

	// Start transaction
	txStore, err := s.store.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rollbackErr := txStore.Rollback(); rollbackErr != nil {
				s.logger.Errorf("Failed to rollback after error: %v (original error: %v)", rollbackErr, err)
			}
			return
		}
		if commitErr := txStore.Commit(); commitErr != nil {
			s.logger.Errorf("Failed to commit: %v", commitErr)
			err = commitErr
		}
	}()

	// Fetch existing workflow to ensure it exists
	wf, err := txStore.GetWorkflow(id)
	if err != nil {
		return err
	}

	// Update status and timestamp
	if err := txStore.UpdateWorkflowStatus(wf.ID, wfStatus); err != nil {
		return err
	}

	s.logger.Infof("Updated workflow ID %d to status '%s'", id, status)
	return nil
}

func (s *WorkflowService) ListWorkflows() ([]models.Workflow, error) {
	return s.store.ListWorkflows()
}

// GetWorkflow fetches a workflow with its tasks
func (s *WorkflowService) GetWorkflow(workflowID int64) (models.Workflow, error) {
	wf, err := s.store.GetWorkflow(workflowID)
	if err != nil {
		return models.Workflow{}, fmt.Errorf("failed to get workflow %d: %v", workflowID, err)
	}
	return wf, nil
}
