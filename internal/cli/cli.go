package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ignatij/goflow/internal/log"
	internal_storage "github.com/ignatij/goflow/internal/storage"
	"github.com/ignatij/goflow/pkg/service"
	"github.com/spf13/cobra"
)

func SetupCLI(rootCmd *cobra.Command) {
	createCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new workflow (CLI)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dbConnStr, err := cmd.Flags().GetString("db")
			if err != nil {
				log.GetLogger().Errorf("Error retrieving db flag: %v", err)
				os.Exit(1)
			}
			log.GetLogger().Debugf("Running create with db: %s", dbConnStr)
			store := initStore(dbConnStr)
			defer store.Close()
			svc := service.NewWorkflowService(store, log.GetLogger())
			if len(args) != 1 {
				log.GetLogger().Errorf("Wrong number of arguments, expected 1 got %v", len(args))
				fmt.Printf("Wrong number of arguments, expected 1 got %v", len(args))
				os.Exit(1)
			}
			name := strings.Split(args[0], "=")[1]
			createWorkflow(svc, name)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all workflows (CLI)",
		Run: func(cmd *cobra.Command, args []string) {
			dbConnStr, err := cmd.Flags().GetString("db")
			if err != nil {
				log.GetLogger().Errorf("Error retrieving db flag: %v", err)
				os.Exit(1)
			}
			log.GetLogger().Debugf("Running list with db: %s", dbConnStr)
			store := initStore(dbConnStr)
			defer store.Close()
			svc := service.NewWorkflowService(store, log.GetLogger())
			listWorkflows(svc)
		},
	}

	updateWorkflowStatusCmd := &cobra.Command{
		Use:   "update",
		Short: "Update a workflow's status",
		Run: func(cmd *cobra.Command, args []string) {
			dbConnStr, err := cmd.Flags().GetString("db")
			if err != nil {
				log.GetLogger().Errorf("Error retrieving db flag: %v", err)
				os.Exit(1)
			}

			if len(args) != 2 {
				log.GetLogger().Errorf("Wrong number of args, expected 2, got %v", len(args))
				fmt.Println("Wrong number of arguments, expected 2")
				os.Exit(1)
			}
			id, err := strconv.Atoi(strings.Split(args[0], "=")[1])
			if err != nil {
				log.GetLogger().Errorf("Error parsing id as number: %v", err)
				fmt.Printf("Error parsing id as number: %v", err)
				os.Exit(1)
			}
			status := strings.Split(args[1], "=")[1]
			if id == 0 || status == "" {
				fmt.Println("Error: id and status are required")
				os.Exit(1)
			}
			store := initStore(dbConnStr)
			defer store.Close()
			svc := service.NewWorkflowService(store, log.GetLogger())
			updateWorkflowStatus(svc, int64(id), status)
		},
	}

	rootCmd.AddCommand(createCmd, listCmd, updateWorkflowStatusCmd)
}

func createWorkflow(svc *service.WorkflowService, name string) {
	id, err := svc.CreateWorkflow(name)
	if err != nil {
		log.GetLogger().Errorf("Failed to create workflow: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to create workflow: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "Created workflow '%s' with ID %d\n", name, id)
}

func updateWorkflowStatus(svc *service.WorkflowService, id int64, status string) {
	err := svc.UpdateWorkflowStatus(id, status)
	if err != nil {
		log.GetLogger().Errorf("Failed to update workflow status: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to update workflow status: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "Updated the status of the workflow with ID %d to '%s'\n", id, status)
}

func listWorkflows(svc *service.WorkflowService) {
	workflows, err := svc.ListWorkflows()
	if err != nil {
		log.GetLogger().Errorf("Failed to list workflows: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to list workflows: %v\n", err)
		os.Exit(1)
	}
	if len(workflows) == 0 {
		fmt.Fprintf(os.Stdout, "No workflows found.\n")
		return
	}
	fmt.Fprintf(os.Stdout, "Workflows:\n")
	for _, wf := range workflows {
		fmt.Fprintf(os.Stdout, "- ID: %d, Name: %s, Status: %s, Created: %s\n",
			wf.ID, wf.Name, wf.Status, wf.CreatedAt.Format(time.RFC3339))
	}
}

func initStore(dbConnStr string) *internal_storage.PostgresStore {
	store, err := internal_storage.InitStore(dbConnStr)
	if err != nil {
		log.GetLogger().Errorf("Failed to initialize store: %v", err)
		os.Exit(1)
	}
	return store
}
