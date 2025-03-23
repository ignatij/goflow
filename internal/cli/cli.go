package cli

import (
	"os"
	"time"

	"github.com/ignatij/goflow/internal/log"
	internal_storage "github.com/ignatij/goflow/internal/storage"
	"github.com/ignatij/goflow/pkg/models"
	"github.com/ignatij/goflow/pkg/storage"
	"github.com/spf13/cobra"
)

type CLI struct {
	store storage.Store
}

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
			cli := &CLI{}
			cli.store = store
			cli.createWorkflow(args)
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
			cli := &CLI{}
			defer store.Close()
			cli.store = store
			cli.listWorkflows()
		},
	}

	rootCmd.AddCommand(createCmd, listCmd)
}

func (c *CLI) createWorkflow(args []string) {
	wf := models.Workflow{
		Name:      args[0],
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	id, err := c.store.SaveWorkflow(wf)
	if err != nil {
		log.GetLogger().Errorf("Failed to create workflow: %v", err)
		os.Exit(1)
	}
	log.GetLogger().Infof("Created workflow '%s' with ID %d", wf.Name, id)
}

func (c *CLI) listWorkflows() {
	workflows, err := c.store.ListWorkflows()
	if err != nil {
		log.GetLogger().Errorf("Failed to list workflows: %v", err)
		os.Exit(1)
	}
	if len(workflows) == 0 {
		log.GetLogger().Info("No workflows found.")
		return
	}
	log.GetLogger().Info("Workflows:")
	for _, wf := range workflows {
		log.GetLogger().Infof("- ID: %d, Name: %s, Status: %s, Created: %s", wf.ID, wf.Name, wf.Status, wf.CreatedAt.Format(time.RFC3339))
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
