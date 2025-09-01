package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
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

	var failCount int32
	err := wfService.RegisterTask("unstable", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		if atomic.AddInt32(&failCount, 1) == 1 {
			fmt.Println("Unstable task: failing on first attempt")
			return nil, fmt.Errorf("simulated failure")
		}
		fmt.Println("Unstable task: succeeded on retry")
		return "success after retry", nil
	}, nil, models.WithRetries(2))
	if err != nil {
		logger.Errorf("Failed to register unstable task: %v", err)
		os.Exit(1)
	}

	err = wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		fmt.Printf("Flow got: %v\n", args)
		return "error handling complete", nil
	}, []string{"unstable"})
	if err != nil {
		logger.Errorf("Failed to register flow: %v", err)
		os.Exit(1)
	}

	workflowID, err := wfService.CreateWorkflow("error-handling-example")
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
