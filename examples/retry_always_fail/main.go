package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
)

func main() {
	ctx := context.Background()
	logger := log.GetLogger()
	store := storage.NewMockStore()

	wfService := service.NewWorkflowService(ctx, store, logger)

	wfService.RegisterTask("always_fail", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Task attempt: always failing!")
		return nil, fmt.Errorf("simulated permanent failure")
	}, nil, models.WithRetries(3))

	wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("This should not be printed if task always fails.")
		return "should not complete", nil
	}, []string{"always_fail"})

	workflowID, err := wfService.CreateWorkflow("retry-always-fail-example")
	if err != nil {
		logger.Errorf("Failed to create workflow: %v", err)
		os.Exit(1)
	}

	_, err = wfService.ExecuteFlow(ctx, workflowID, "main")
	if err == nil {
		fmt.Println("Expected workflow to fail, but it succeeded!")
		os.Exit(1)
	}

	logger.Errorf("Workflow execution failed as expected: %v", err)
	fmt.Println("Done.")
	time.Sleep(100 * time.Millisecond)
}
