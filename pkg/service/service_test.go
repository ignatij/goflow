package service_test

import (
	"fmt"
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
	// no-op
}

func (l logger) Errorf(format string, args ...interface{}) {
	// no-op
}

func TestClientWorkflowInMemory_Pipeline(t *testing.T) {

	newWorkflowService := func() *service.WorkflowService {
		return service.NewWorkflowService(storage.NewMockStore(), logger{})
	}

	t.Run("UnregisteredFlow", func(t *testing.T) {
		svc := newWorkflowService()
		wfID, err := svc.CreateWorkflow("test")
		assert.NoError(t, err)
		_, err = svc.ExecuteFlow(wfID, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "flow 'nonexistent' is not registered!")
	})

	t.Run("UnregisteredTaskInFlow", func(t *testing.T) {
		svc := newWorkflowService()

		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}

		// Register flow with non-existent task dependency
		err := svc.RegisterFlow("pipeline", pipelineFlow, []string{"nonexistent"})
		assert.NoError(t, err)

		wfID, err := svc.CreateWorkflow("testUnregisteredTask")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		svc := newWorkflowService() // or svc := service.NewWorkflowService(postgresStore(t), logger{})

		fetchTask1 := func() (service.TaskResult, error) {
			return "raw data 1", nil
		}
		fetchTask2 := func() (service.TaskResult, error) {
			return "raw data 2", nil
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			str, ok := data.(string)
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

		result, err := svc.ExecuteFlow(wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: raw data 2", result) // Uses fetchTask2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateTask", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1) // Single task persisted due to SaveTask logic
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
	})

	t.Run("DuplicateFlowRegistration", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow1 := func(args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed v1: " + str, nil
		}
		pipelineFlow2 := func(args ...service.TaskResult) (service.TaskResult, error) {
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

		result, err := svc.ExecuteFlow(wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed v2: raw data", result) // Uses pipelineFlow2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateFlow", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1)
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
	})

	t.Run("InvalidTaskDependencies", func(t *testing.T) {
		svc := newWorkflowService()

		processTask := func(args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"nonexistent"}))
		pipelineFlow := func(args ...service.TaskResult) (service.TaskResult, error) {
			return args[0], nil
		}
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("testInvalidDeps")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency 'nonexistent' for 'process' not registered")
		assert.Nil(t, result)

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testInvalidDeps", wf.Name)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status) // Adjust if FAILED
		assert.Len(t, wf.Tasks, 0)                              // No tasks persisted
	})

	t.Run("EmptyTaskFlowRegistration", func(t *testing.T) {
		svc := newWorkflowService() // or service.NewWorkflowService(postgresStore(t), logger{})

		err := svc.RegisterTask("", func() (service.TaskResult, error) {
			return "data", nil
		}, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty task name")

		err = svc.RegisterFlow("", func() (service.TaskResult, error) {
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

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			str, ok := data.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", data)
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
	})

	t.Run("MoreComplexPipeline", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		processTask := func(fetchTaskResult service.TaskResult) (service.TaskResult, error) {
			return "processing: " + fetchTaskResult.(string), nil
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
		assert.Equal(t, "COMPLETED", wf.Tasks[1].Status)
	})

	t.Run("MultipleDependencies", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		processTask := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}
		pipelineFlow := func(fetched, processed service.TaskResult) (service.TaskResult, error) {
			return fetched.(string) + " | " + processed.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch", "process"}))

		wfID, err := svc.CreateWorkflow("multiDepPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
		assert.Equal(t, "process", wf.Tasks[1].ID)
		assert.Equal(t, "COMPLETED", wf.Tasks[1].Status)
	})

	t.Run("MultipleFlows", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		cleanTask := func(data service.TaskResult) (service.TaskResult, error) {
			return "cleaned: " + data.(string), nil
		}
		preprocessFlow := func(cleaned service.TaskResult) (service.TaskResult, error) {
			return cleaned, nil
		}
		analyzeTask := func(data service.TaskResult) (service.TaskResult, error) {
			return "analyzed: " + data.(string), nil
		}
		reportFlow := func(analyzed service.TaskResult) (service.TaskResult, error) {
			return "report: " + analyzed.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("clean", cleanTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("preprocess", preprocessFlow, []string{"clean"}))
		assert.NoError(t, svc.RegisterTask("analyze", analyzeTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("report", reportFlow, []string{"analyze"}))

		wfID, err := svc.CreateWorkflow("multiFlowPipeline")
		assert.NoError(t, err)

		// Execute preprocess flow
		preprocessResult, err := svc.ExecuteFlow(wfID, "preprocess")
		assert.NoError(t, err)
		assert.Equal(t, "cleaned: raw data", preprocessResult)

		// Execute report flow on the same workflow
		reportResult, err := svc.ExecuteFlow(wfID, "report")
		assert.NoError(t, err)
		assert.Equal(t, "report: analyzed: raw data", reportResult)

		// Verify workflow status and tasks
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 3) // "fetch", "clean", "analyze" (fetch reused)
		for _, task := range wf.Tasks {
			assert.Equal(t, "COMPLETED", task.Status)
		}
		assert.Contains(t, []string{"fetch", "clean", "analyze"}, wf.Tasks[0].ID)
		assert.Contains(t, []string{"fetch", "clean", "analyze"}, wf.Tasks[1].ID)
		assert.Contains(t, []string{"fetch", "clean", "analyze"}, wf.Tasks[2].ID)
	})

	t.Run("FailureHandling", func(t *testing.T) {
		svc := newWorkflowService()

		fetchTask := func() (service.TaskResult, error) {
			return "data", nil
		}
		failTask := func(data service.TaskResult) (service.TaskResult, error) {
			return nil, errors.New("task failed")
		}
		flow := func(data service.TaskResult) (service.TaskResult, error) {
			return data, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("fail", failTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("testFlow", flow, []string{"fail"}))

		wfID, err := svc.CreateWorkflow("failTest")
		assert.NoError(t, err)

		_, err = svc.ExecuteFlow(wfID, "testFlow")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "execution of 'fail' failed: task failed")

		// Verify workflow and task status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 2)
		for _, task := range wf.Tasks {
			if task.ID == "fetch" {
				assert.Equal(t, "COMPLETED", task.Status)
			}
			if task.ID == "fail" {
				assert.Equal(t, "FAILED", task.Status)
				assert.Equal(t, "task failed", task.ErrorMsg)
			}
		}
	})

	t.Run("MultipleRuns", func(t *testing.T) {
		svc := newWorkflowService()
		fetchTask := func() (service.TaskResult, error) {
			return time.Now().String(), nil // Dynamic result
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		// First run
		wfID1, err := svc.CreateWorkflow("run1")
		assert.NoError(t, err)
		result1, err := svc.ExecuteFlow(wfID1, "pipeline")
		assert.NoError(t, err)

		// Second run
		wfID2, err := svc.CreateWorkflow("run2")
		assert.NoError(t, err)
		result2, err := svc.ExecuteFlow(wfID2, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf1.Tasks[0].Status)

		wf2, err := svc.GetWorkflow(wfID2)
		assert.NoError(t, err)
		assert.Len(t, wf2.Tasks, 1)
		assert.Equal(t, "fetch", wf2.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", wf2.Tasks[0].Status)
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
		svc := service.NewWorkflowService(postgresStore(t), logger{})
		wfID, err := svc.CreateWorkflow("test")
		assert.NoError(t, err)
		_, err = svc.ExecuteFlow(wfID, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "flow 'nonexistent' is not registered!")
	})

	t.Run("UnregisteredTaskInFlow", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})

		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}

		// Register flow with non-existent task dependency
		err := svc.RegisterFlow("pipeline", pipelineFlow, []string{"nonexistent"})
		assert.NoError(t, err)

		wfID, err := svc.CreateWorkflow("testUnregisteredTask")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		svc := service.NewWorkflowService(postgresStore(t), logger{})

		fetchTask1 := func() (service.TaskResult, error) {
			return "raw data 1", nil
		}
		fetchTask2 := func() (service.TaskResult, error) {
			return "raw data 2", nil
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			str, ok := data.(string)
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

		result, err := svc.ExecuteFlow(wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed: raw data 2", result) // Uses fetchTask2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateTask", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1) // Single task persisted due to SaveTask logic
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
	})

	t.Run("DuplicateFlowRegistration", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow1 := func(args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed v1: " + str, nil
		}
		pipelineFlow2 := func(args ...service.TaskResult) (service.TaskResult, error) {
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

		result, err := svc.ExecuteFlow(wfID, "pipeline")
		assert.NoError(t, err)
		assert.Equal(t, "processed v2: raw data", result) // Uses pipelineFlow2

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testDuplicateFlow", wf.Name)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 1)
		assert.Equal(t, "fetch", wf.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
	})

	t.Run("InvalidTaskDependencies", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})

		processTask := func(args ...service.TaskResult) (service.TaskResult, error) {
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", args[0])
			}
			return "processed: " + str, nil
		}

		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"nonexistent"}))
		pipelineFlow := func(args ...service.TaskResult) (service.TaskResult, error) {
			return args[0], nil
		}
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("testInvalidDeps")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency 'nonexistent' for 'process' not registered")
		assert.Nil(t, result)

		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, "testInvalidDeps", wf.Name)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status) // Adjust if FAILED
		assert.Len(t, wf.Tasks, 0)                              // No tasks persisted
	})

	t.Run("EmptyTaskFlowRegistration", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})

		err := svc.RegisterTask("", func() (service.TaskResult, error) {
			return "data", nil
		}, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty task name")

		err = svc.RegisterFlow("", func() (service.TaskResult, error) {
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
		svc := service.NewWorkflowService(postgresStore(t), logger{})

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
	})

	t.Run("MoreComplexPipeline", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})

		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		processTask := func(fetchTaskResult service.TaskResult) (service.TaskResult, error) {
			return "processing: " + fetchTaskResult.(string), nil
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"process"}))

		wfID, err := svc.CreateWorkflow("dataPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
		assert.Equal(t, "COMPLETED", wf.Tasks[1].Status)
	})

	t.Run("MultipleDependencies", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})
		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		processTask := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}
		pipelineFlow := func(fetched, processed service.TaskResult) (service.TaskResult, error) {
			return fetched.(string) + " | " + processed.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("process", processTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch", "process"}))

		wfID, err := svc.CreateWorkflow("multiDepPipeline")
		assert.NoError(t, err)

		result, err := svc.ExecuteFlow(wfID, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf.Tasks[0].Status)
		assert.Equal(t, "process", wf.Tasks[1].ID)
		assert.Equal(t, "COMPLETED", wf.Tasks[1].Status)
	})

	t.Run("MultipleFlows", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})
		fetchTask := func() (service.TaskResult, error) {
			return "raw data", nil
		}
		cleanTask := func(data service.TaskResult) (service.TaskResult, error) {
			return "cleaned: " + data.(string), nil
		}
		preprocessFlow := func(cleaned service.TaskResult) (service.TaskResult, error) {
			return cleaned, nil
		}
		analyzeTask := func(data service.TaskResult) (service.TaskResult, error) {
			return "analyzed: " + data.(string), nil
		}
		reportFlow := func(analyzed service.TaskResult) (service.TaskResult, error) {
			return "report: " + analyzed.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("clean", cleanTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("preprocess", preprocessFlow, []string{"clean"}))
		assert.NoError(t, svc.RegisterTask("analyze", analyzeTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("report", reportFlow, []string{"analyze"}))

		wfID, err := svc.CreateWorkflow("multiFlowPipeline")
		assert.NoError(t, err)

		// Execute preprocess flow
		preprocessResult, err := svc.ExecuteFlow(wfID, "preprocess")
		assert.NoError(t, err)
		assert.Equal(t, "cleaned: raw data", preprocessResult)

		// Execute report flow on the same workflow
		reportResult, err := svc.ExecuteFlow(wfID, "report")
		assert.NoError(t, err)
		assert.Equal(t, "report: analyzed: raw data", reportResult)

		// Verify workflow status and tasks
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.CompletedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 3)
		taskIDs := make(map[string]bool)
		for _, task := range wf.Tasks {
			assert.Equal(t, "COMPLETED", task.Status)
			taskIDs[task.ID] = true
		}
		assert.Len(t, taskIDs, 3, "Expected exactly 3 unique tasks")
		assert.True(t, taskIDs["fetch"], "Expected fetch task")
		assert.True(t, taskIDs["clean"], "Expected clean task")
		assert.True(t, taskIDs["analyze"], "Expected analyze task")
	})

	t.Run("FailureHandling", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})
		fetchTask := func() (service.TaskResult, error) {
			return "data", nil
		}
		failTask := func(data service.TaskResult) (service.TaskResult, error) {
			return nil, errors.New("task failed")
		}
		flow := func(data service.TaskResult) (service.TaskResult, error) {
			return data, nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterTask("fail", failTask, []string{"fetch"}))
		assert.NoError(t, svc.RegisterFlow("testFlow", flow, []string{"fail"}))

		wfID, err := svc.CreateWorkflow("failTest")
		assert.NoError(t, err)

		_, err = svc.ExecuteFlow(wfID, "testFlow")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "execution of 'fail' failed: task failed")

		// Verify workflow and task status
		wf, err := svc.GetWorkflow(wfID)
		assert.NoError(t, err)
		assert.Equal(t, models.FailedWorkflowStatus, wf.Status)
		assert.Len(t, wf.Tasks, 2)
		for _, task := range wf.Tasks {
			if task.ID == "fetch" {
				assert.Equal(t, "COMPLETED", task.Status)
			}
			if task.ID == "fail" {
				assert.Equal(t, "FAILED", task.Status)
				assert.Equal(t, "task failed", task.ErrorMsg)
			}
		}
	})

	t.Run("MultipleRuns", func(t *testing.T) {
		svc := service.NewWorkflowService(postgresStore(t), logger{})
		fetchTask := func() (service.TaskResult, error) {
			return time.Now().String(), nil // Dynamic result
		}
		pipelineFlow := func(data service.TaskResult) (service.TaskResult, error) {
			return "processed: " + data.(string), nil
		}

		assert.NoError(t, svc.RegisterTask("fetch", fetchTask, []string{}))
		assert.NoError(t, svc.RegisterFlow("pipeline", pipelineFlow, []string{"fetch"}))

		// First run
		wfID1, err := svc.CreateWorkflow("run1")
		assert.NoError(t, err)
		result1, err := svc.ExecuteFlow(wfID1, "pipeline")
		assert.NoError(t, err)

		// Second run
		wfID2, err := svc.CreateWorkflow("run2")
		assert.NoError(t, err)
		result2, err := svc.ExecuteFlow(wfID2, "pipeline")
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
		assert.Equal(t, "COMPLETED", wf1.Tasks[0].Status)

		wf2, err := svc.GetWorkflow(wfID2)
		assert.NoError(t, err)
		assert.Len(t, wf2.Tasks, 1)
		assert.Equal(t, "fetch", wf2.Tasks[0].ID)
		assert.Equal(t, "COMPLETED", wf2.Tasks[0].Status)
	})

}
