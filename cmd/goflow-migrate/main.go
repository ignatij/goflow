// cmd/goflow-migrate/main.go
package main

import (
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{Use: "goflow-migrate"}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	Run: func(cmd *cobra.Command, args []string) {
		// Load .env if present
		if err := godotenv.Load(); err != nil {
			fmt.Printf("No .env file found or failed to load: %v. Using --db flag.\n", err)
		}

		connStr, _ := cmd.Flags().GetString("db")
		if connStr == "" {
			// Fallback to constructing from env vars if --db not provided
			dbUsername := os.Getenv("DB_USERNAME")
			dbPassword := os.Getenv("DB_PASSWORD")
			dbHost := os.Getenv("DB_HOST")
			dbPort := os.Getenv("DB_PORT")
			dbName := os.Getenv("DB_NAME")
			if dbUsername == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" {
				fmt.Println("Error: --db flag or complete DB_* env vars (DB_USERNAME, DB_PASSWORD, DB_HOST, DB_PORT, DB_NAME) required")
				os.Exit(1)
			}
			connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
				dbUsername, dbPassword, dbHost, dbPort, dbName)
		}

		m, err := migrate.New("file://migrations", connStr)
		if err != nil {
			fmt.Printf("Failed to initialize migrations: %v\n", err)
			os.Exit(1)
		}
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			fmt.Printf("Failed to apply migrations: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Migrations applied successfully")
	},
}

func main() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().String("db", "", "Database connection string (optional if DB_* env vars are set)")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
