package main

import (
	"fmt"
	"os"

	"github.com/ignatij/goflow/internal/cli"
	"github.com/ignatij/goflow/internal/http"
	"github.com/ignatij/goflow/internal/log"
	"github.com/ignatij/goflow/internal/storage"
	"github.com/spf13/cobra"
)

func main() {
	var dbConnStr, port string

	rootCmd := &cobra.Command{
		Use:   "goflow",
		Short: "GoFlow: A workflow management server",
		Long:  "GoFlow manages workflows via HTTP API, with optional CLI commands.",
		Run: func(cmd *cobra.Command, args []string) {
			log.GetLogger().Infof("Running server with db: %s, port: %s", dbConnStr, port)
			store := initStore(dbConnStr)
			defer store.Close()
			if err := http.StartServer(port, store); err != nil {
				fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
				os.Exit(1)
			}
		},
	}
	rootCmd.PersistentFlags().StringVar(&dbConnStr, "db", fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USERNAME"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME")), "Database connection string")
	rootCmd.PersistentFlags().StringVar(&port, "port", "8080", "HTTP server port")

	// Add CLI subcommands to rootCmd
	cli.SetupCLI(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		log.GetLogger().Errorf("Error executing command: %v", err)
		os.Exit(1)
	}
}

func initStore(dbConnStr string) *storage.PostgresStore {
	store, err := storage.InitStoreAndRunMigrations(dbConnStr)
	if err != nil {
		log.GetLogger().Errorf("Failed to initialize store: %v", err)
		os.Exit(1)
	}
	return store
}
