package service_test

import (
	"testing"

	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
)

type logger struct{}

func (l logger) Infof(format string, args ...interface{}) {
	// no-op
}

func (l logger) Errorf(format string, args ...interface{}) {
	// no-op
}

func newWorkflowService() *service.WorkflowService {
	return service.NewWorkflowService(storage.NewMockStore(), logger{})
}

// TestRegisterTask tests task registration and validation
func TestRegisterTask(t *testing.T) {
	tests := []struct {
		name    string
		fn      interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid task",
			fn: func() (service.TaskResult, error) {
				return "result", nil
			},
			wantErr: false,
		},
		{
			name:    "invalid - not a function",
			fn:      "not a function",
			wantErr: true,
			errMsg:  "invalid task function for 'invalid - not a function': must be a function",
		},
		{
			name: "invalid - wrong return count",
			fn: func() error {
				return nil
			},
			wantErr: true,
			errMsg:  "invalid task function for 'invalid - wrong return count': must return (TaskResult, error)",
		},
		{
			name: "invalid - wrong second return type",
			fn: func() (string, string) {
				return "result", "error"
			},
			wantErr: true,
			errMsg:  "invalid task function for 'invalid - wrong second return type': must return (TaskResult, error)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newWorkflowService()
			err := svc.RegisterTask(tt.name, tt.fn)
			if tt.wantErr {
				if err == nil || err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %v", tt.errMsg, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
		})
	}
}

// TestRegisterFlow tests flow registration and validation
func TestRegisterFlow(t *testing.T) {
	tests := []struct {
		name    string
		fn      interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid flow",
			fn: func(data service.TaskResult) (service.TaskResult, error) {
				return data, nil
			},
			wantErr: false,
		},
		{
			name:    "invalid - not a function",
			fn:      42,
			wantErr: true,
			errMsg:  "invalid flow function for 'invalid - not a function': must be a function",
		},
		{
			name:    "invalid - no returns",
			fn:      func() {},
			wantErr: true,
			errMsg:  "invalid flow function for 'invalid - no returns': must return (TaskResult, error)",
		},
		{
			name: "valid - flexible TaskResult",
			fn: func() (service.TaskResult, error) {
				return 42, nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newWorkflowService()
			err := svc.RegisterFlow(tt.name, tt.fn)
			if tt.wantErr {
				if err == nil || err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %v", tt.errMsg, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
		})
	}
}

// TestIntegration_CreateAndRegister tests workflow creation and task registration together
func TestIntegration_CreateAndRegister(t *testing.T) {
	svc := newWorkflowService()

	// Create a workflow
	id, err := svc.CreateWorkflow("testWorkflow")
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive workflow ID, got %d", id)
	}

	// Register a task
	taskFn := func() (service.TaskResult, error) {
		return "task result", nil
	}
	if err := svc.RegisterTask("testTask", taskFn); err != nil {
		t.Errorf("RegisterTask failed: %v", err)
	}

	// Verify workflow in store
	workflows, err := svc.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}
	if len(workflows) != 1 || workflows[0].ID != id || workflows[0].Name != "testWorkflow" {
		t.Errorf("expected workflow 'testWorkflow' with ID %d, got %v", id, workflows)
	}
}
