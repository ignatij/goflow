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

	wfService.RegisterTask("timeout_task", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Task attempt: will timeout!")
		select {
		case <-time.After(2 * time.Second):
			return "should not reach here", nil
		case <-ctx.Done():
			fmt.Println("Task cancelled due to timeout!")
			return nil, ctx.Err()
		}
	}, nil, models.WithRetries(2), models.WithTimeout(1*time.Second))

	wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("This should not be printed if task always times out.")
		return "should not complete", nil
	}, []string{"timeout_task"})

	workflowID, err := wfService.CreateWorkflow("retry-with-timeout-example")
	if err != nil {
		logger.Errorf("Failed to create workflow: %v", err)
		os.Exit(1)
	}

	_, err = wfService.ExecuteFlow(ctx, workflowID, "main")
	if err == nil {
		fmt.Println("Expected workflow to fail due to timeout, but it succeeded!")
		os.Exit(1)
	}

	logger.Errorf("Workflow execution failed as expected: %v", err)
	fmt.Println("Done.")
	time.Sleep(100 * time.Millisecond)
}
