package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/internal/service"
	internal_storage "github.com/ignatij/goflow/internal/storage"
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
			svc := service.NewWorkflowService(store)
			createWorkflow(svc, args)
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
			svc := service.NewWorkflowService(store)
			listWorkflows(svc)
		},
	}

	rootCmd.AddCommand(createCmd, listCmd)
}

func createWorkflow(svc *service.WorkflowService, args []string) {
	id, err := svc.CreateWorkflow(args[0])
	if err != nil {
		log.GetLogger().Errorf("Failed to create workflow: %v", err)
		fmt.Fprintf(os.Stderr, "Error: failed to create workflow: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "Created workflow '%s' with ID %d\n", args[0], id)
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
