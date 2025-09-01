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
	logger := log.GetLogger()
	store := storage.NewMockStore()

	wfService := service.NewWorkflowService(context.Background(), store, logger)

	err := wfService.RegisterTask("slow", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Starting slow task...")
		select {
		case <-time.After(3 * time.Second):
			fmt.Println("Slow task finished!")
			return "done", nil
		case <-ctx.Done():
			fmt.Println("Slow task cancelled due to context timeout!")
			return nil, ctx.Err()
		}
	}, nil)
	if err != nil {
		logger.Errorf("Failed to register task: %v", err)
		os.Exit(1)
	}

	err = wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Flow finished (should not reach here if timeout)")
		return "flow complete", nil
	}, []string{"slow"})
	if err != nil {
		logger.Errorf("Failed to register flow: %v", err)
		os.Exit(1)
	}

	workflowID, err := wfService.CreateWorkflow("context-timeout-example")
	if err != nil {
		logger.Errorf("Failed to create workflow: %v", err)
		os.Exit(1)
	}

	// Set a timeout shorter than the task duration
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = wfService.ExecuteFlow(timeoutCtx, workflowID, "main")
	if err == nil {
		fmt.Printf("Expected timeout error: %v\n", err)
		os.Exit(1)
	}

	logger.Errorf("Workflow execution failed: %v", err)
	fmt.Println("Done.")
	// Give time for async logs to flush (if any)
	time.Sleep(100 * time.Millisecond)
}
