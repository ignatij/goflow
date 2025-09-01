package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	err := wfService.RegisterTask("sample input", service.WrapTaskFunc(func(args ...service.TaskResult) (service.TaskResult, error) {
		return "test", nil
	}), nil)
	if err != nil {
		logger.Errorf("Failed to register sample input task: %v", err)
		os.Exit(1)
	}

	err = wfService.RegisterTask("capitalize", service.WrapTaskFunc(func(args ...service.TaskResult) (service.TaskResult, error) {
		input := "goflow"
		if len(args) > 0 {
			if s, ok := args[0].(string); ok {
				input = s
			}
		}
		result := strings.ToUpper(input)
		fmt.Printf("Capitalized '%s' to '%s'\n", input, result)
		return result, nil
	}),
		// replace with nil if you want to set to default behavior of capitalizing 'goflow'
		[]string{"sample input"})
	if err != nil {
		logger.Errorf("Failed to register capitalize task: %v", err)
		os.Exit(1)
	}

	err = wfService.RegisterFlow("main", service.WrapTaskFunc(func(args ...service.TaskResult) (service.TaskResult, error) {
		if len(args) > 0 {
			fmt.Printf("Flow received: %v\n", args[0])
		}
		return "custom task complete", nil
	}), []string{"capitalize"})
	if err != nil {
		logger.Errorf("Failed to register main flow: %v", err)
		os.Exit(1)
	}

	workflowID, err := wfService.CreateWorkflow("custom-task-example")
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
