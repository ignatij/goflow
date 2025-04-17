package service

import (
	"fmt"
	"reflect"
	"slices"
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
type TaskFunc interface{}

// WorkflowService manages workflow instances and their flow executions.
// A workflow is a specific instance of a pipeline run, persisted with a unique ID.
// A flow is a reusable definition of tasks and dependencies, executed within a workflow.
type WorkflowService struct {
	store    storage.Store
	logger   Logger
	tasks    map[string]TaskFunc
	taskDeps map[string][]string
	flows    map[string]TaskFunc
	flowDeps map[string][]string
	results  map[int64]map[string]TaskResult
}

func NewWorkflowService(store storage.Store, logger Logger) *WorkflowService {
	return &WorkflowService{
		store:    store,
		logger:   logger,
		tasks:    make(map[string]TaskFunc),
		taskDeps: make(map[string][]string),
		flows:    make(map[string]TaskFunc),
		flowDeps: make(map[string][]string),
		results:  make(map[int64]map[string]TaskResult)}
}

// RegisterTask registers a task, inferring dependencies from parameter names
func (s *WorkflowService) RegisterTask(name string, fn TaskFunc, deps []string) error {
	if err := validateTaskFunc(fn); err != nil {
		return fmt.Errorf("invalid task function for '%s': %v", name, err)
	}
	if len(name) == 0 {
		return errors.New("empty task name")
	}
	fnType := reflect.TypeOf(fn)
	if fnType.NumIn() != len(deps) {
		return fmt.Errorf("task '%s' has %d dependencies but function expects %d arguments", name, len(deps), fnType.NumIn())
	}
	s.tasks[name] = fn
	s.taskDeps[name] = deps
	s.logger.Infof("Registered task '%s' with dependencies '%v'", name, deps)
	return nil
}

// RegisterFlow registers a flow, inferring dependencies from parameter names
func (s *WorkflowService) RegisterFlow(name string, fn TaskFunc, deps []string) error {
	if len(name) == 0 {
		return errors.New("empty flow name")
	}
	if err := validateTaskFunc(fn); err != nil {
		return fmt.Errorf("invalid flow function for '%s': %v", name, err)
	}
	fnType := reflect.TypeOf(fn)
	if fnType.NumIn() != len(deps) {
		return fmt.Errorf("flow '%s' has %d dependencies but function expects %d arguments", name, len(deps), fnType.NumIn())
	}
	s.flows[name] = fn
	s.flowDeps[name] = deps
	s.logger.Infof("Registered flow '%s' with dependencies %v", name, deps)
	return nil
}

// validateTaskFunc checks if the function matches the expected signature
func validateTaskFunc(fn TaskFunc) error {
	fnType := reflect.TypeOf(fn)
	if fnType == nil || fnType.Kind() != reflect.Func {
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
func (s *WorkflowService) ExecuteFlow(workflowID int64, flowName string) (TaskResult, error) {
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

	if _, ok := s.flows[flowName]; !ok {
		return nil, fmt.Errorf("flow '%s' is not registered!", flowName)
	}

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
	if _, ok := s.results[workflowID]; !ok {
		s.results[workflowID] = make(map[string]TaskResult)
	}

	// Execute tasks and flows in order
	for _, name := range order {
		var fn TaskFunc
		var isTask bool
		if f, ok := s.tasks[name]; ok {
			fn = f
			isTask = true
		} else if f, ok := s.flows[name]; ok {
			fn = f
			isTask = false
		} else {
			continue // Shouldn't happen due to topological sort
		}

		deps := s.taskDeps[name]
		if !isTask {
			deps = s.flowDeps[name]
		}

		// Prepare arguments from dependencies
		args := make([]reflect.Value, len(deps))
		for i, dep := range deps {
			if result, ok := s.results[workflowID][dep]; ok {
				args[i] = reflect.ValueOf(result)
			} else {
				err = fmt.Errorf("missing result for dependency '%s' of '%s'", dep, name)
				errU := txStore.UpdateWorkflowStatus(workflowID, models.FailedWorkflowStatus)
				if errU != nil {
					return nil, errors.Wrap(errU, fmt.Sprintf("failed to update workflow status after dependency error: %v", err))
				}
				return nil, err
			}
		}

		// Persist task state
		if isTask {
			now := time.Now()
			task := models.Task{
				ID:         name,
				WorkflowID: workflowID,
				Name:       name,
				Status:     "RUNNING",
				StartedAt:  &now,
			}
			if err := txStore.SaveTask(task); err != nil {
				errU := txStore.UpdateWorkflowStatus(workflowID, models.FailedWorkflowStatus)
				if errU != nil {
					return nil, errors.Wrap(errU, fmt.Sprintf("failed to update workflow status after dependency error: %v", err))
				}
				return nil, fmt.Errorf("failed to save task '%s': %v", name, err)
			}
		}

		// Execute the task or flow
		results := reflect.ValueOf(fn).Call(args)
		result, errVal := results[0].Interface(), results[1].Interface()
		if errVal != nil {
			err, ok := errVal.(error)
			if !ok {
				return nil, fmt.Errorf("expected error type but got %T", errVal)
			}
			if isTask {
				errU := txStore.UpdateTaskStatus(name, workflowID, "FAILED", err.Error())
				if errU != nil {
					return nil, errors.Wrap(errU, fmt.Sprintf("failed to update task status after execution flow error: %v", err))
				}
			}
			errU := txStore.UpdateWorkflowStatus(workflowID, models.FailedWorkflowStatus)
			if errU != nil {
				return nil, errors.Wrap(errU, fmt.Sprintf("failed to update worfklow status after execution flow error: %v", err))
			}
			return nil, fmt.Errorf("execution of '%s' failed: %v", name, err)
		}

		// Store result and update task status
		s.results[workflowID][name] = result
		if isTask {
			if err := txStore.UpdateTaskStatus(name, workflowID, "COMPLETED", ""); err != nil {
				errU := txStore.UpdateWorkflowStatus(workflowID, models.FailedWorkflowStatus)
				if errU != nil {
					return nil, errors.Wrap(errU, fmt.Sprintf("failed to update workflow status after failing to update task status with error: %v", err))
				}
				return nil, fmt.Errorf("failed to update task '%s' status: %v", name, err)
			}
		}
	}

	// Mark workflow as completed
	if err := txStore.UpdateWorkflowStatus(workflowID, models.CompletedWorkflowStatus); err != nil {
		return nil, fmt.Errorf("failed to set workflow %d to COMPLETED: %v", workflowID, err)
	}

	finalResult, ok := s.results[workflowID][flowName]
	if !ok {
		return nil, fmt.Errorf("no result for flow '%s'", flowName)
	}
	s.logger.Infof("Executed flow '%s' for workflow %d", flowName, workflowID)
	return finalResult, nil
}

// topologicalSort computes the execution order for a flow
func (s *WorkflowService) topologicalSort(flowName string) ([]string, error) {
	graph := make(map[string][]string)
	inDegree := make(map[string]int)
	allNodes := make(map[string]struct{})

	// build graph
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
			err = commitErr // Update the named return value
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
