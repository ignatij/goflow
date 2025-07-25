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

	wfService := service.NewWorkflowService(ctx, store, logger)

	wfService.RegisterTask("taskA", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Running Task A")
		return "A", nil
	}, nil)

	wfService.RegisterTask("taskB", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Running Task B")
		return "B", nil
	}, nil)

	wfService.RegisterTask("taskC", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Printf("Running Task C, got: %v\n", args)
		return fmt.Sprintf("C(%v, %v)", args[0], args[1]), nil
	}, []string{"taskA", "taskB"})

	wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Printf("Flow received: %v\n", args)
		return "workflow with dependencies complete", nil
	}, []string{"taskC"})

	workflowID, err := wfService.CreateWorkflow("dependency-example")
	if err != nil {
		logger.Errorf("Failed to create workflow: %v", err)
		os.Exit(1)
	}

	result, err := wfService.ExecuteFlow(ctx, workflowID, "main")
	if err != nil {
		logger.Errorf("Workflow execution failed: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Workflow result: %v\n", result)
	fmt.Println("Done.")
	time.Sleep(100 * time.Millisecond)
}
