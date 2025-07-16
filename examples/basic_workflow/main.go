package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
)

func main() {
	ctx := context.Background()
	logger := log.GetLogger()
	store := storage.NewMockStore()

	// Create the workflow service
	wfService := service.NewWorkflowService(ctx, store, logger)

	// Register tasks
	wfService.RegisterTask("hello", service.WrapTaskFunc(func(args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Hello, world!")
		return "hello done", nil
	}), nil)

	wfService.RegisterTask("goodbye", service.WrapTaskFunc(func(args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Goodbye, world!")
		return "goodbye done", nil
	}), []string{"hello"}) // Depends on 'hello'

	// Register a flow (entry point)
	wfService.RegisterFlow("main", service.WrapTaskFunc(func(args ...service.TaskResult) (service.TaskResult, error) {
		return "workflow complete", nil
	}), []string{"goodbye"})

	// Create a workflow instance
	workflowID, err := wfService.CreateWorkflow("basic-example")
	if err != nil {
		logger.Errorf("Failed to create workflow: %v", err)
		os.Exit(1)
	}

	// Execute the workflow
	result, err := wfService.ExecuteFlow(ctx, workflowID, "main")
	if err != nil {
		logger.Errorf("Workflow execution failed: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Workflow result: %v\n", result)
	fmt.Println("Done.")

	// Give time for async logs to flush (if any)
	time.Sleep(100 * time.Millisecond)
}
