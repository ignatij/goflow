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

	ctx := context.Background()
	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	wfService := service.NewWorkflowService(timeoutCtx, store, logger)

	wfService.RegisterTask("slow", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
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

	wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Println("Flow finished (should not reach here if timeout)")
		return "flow complete", nil
	}, []string{"slow"})

	workflowID, err := wfService.CreateWorkflow("context-timeout-example")
	if err != nil {
		logger.Errorf("Failed to create workflow: %v", err)
		os.Exit(1)
	}

	_, err = wfService.ExecuteFlow(ctx, workflowID, "main")
	if err == nil {
		fmt.Printf("Expected timeout error: %v\n", err)
		os.Exit(1)
	}

	logger.Errorf("Workflow execution failed: %v", err)
	fmt.Println("Done.")
	// Give time for async logs to flush (if any)
	time.Sleep(100 * time.Millisecond)
}
