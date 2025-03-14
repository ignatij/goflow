// internal/testutil/db.go
package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDB holds the test database connection and container
type TestDB struct {
	DB        *sqlx.DB
	container testcontainers.Container
}

// SetupTestDB initializes a PostgreSQL container and returns a connected DB
func SetupTestDB(t *testing.T) *TestDB {
	ctx := context.Background()

	// Load .env file
	if err := godotenv.Load(); err != nil {
		t.Logf("No .env file found or failed to load: %v. Proceeding with environment variables.", err)
	}

	// Get environment variables
	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")

	// Validate required variables
	if dbUsername == "" || dbPassword == "" || dbName == "" || dbHost == "" {
		t.Fatalf("Missing required environment variables: DB_USERNAME, DB_PASSWORD, DB_NAME, DB_HOST")
	}

	// Define the container request
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     dbUsername,
			"POSTGRES_PASSWORD": dbPassword,
			"POSTGRES_DB":       dbName,
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(30 * time.Second),
	}

	// Start the container
	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}

	// Connection string
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUsername, dbPassword, dbHost, port.Port(), dbName)

	// Connect to the test DB
	db, err := sqlx.Open("postgres", connStr)
	if err != nil {
		errTerminateCnt := pgContainer.Terminate(ctx)
		if errTerminateCnt != nil {
			t.Fatalf("Failed to terminate container: %v", errTerminateCnt)
		}
		t.Fatalf("Failed to connect to test DB: %v", err)
	}

	// Wait for DB to be ready
	for i := 0; i < 10; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		if i == 9 {
			errTerminateCnt := pgContainer.Terminate(ctx)
			if errTerminateCnt != nil {
				t.Fatalf("Failed to terminate container: %v", errTerminateCnt)
			}
			t.Fatalf("Failed to ping test DB after retries: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Run migrations
	m, err := migrate.New("file://../../migrations", connStr)
	if err != nil {
		errTerminateCnt := pgContainer.Terminate(ctx)
		if errTerminateCnt != nil {
			t.Fatalf("Failed to terminate container: %v", errTerminateCnt)
		}
		t.Fatalf("Failed to initialize migrations: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		errTerminateCnt := pgContainer.Terminate(ctx)
		if errTerminateCnt != nil {
			t.Fatalf("Failed to terminate container: %v", errTerminateCnt)
		}
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	return &TestDB{
		DB:        db,
		container: pgContainer,
	}
}

// Teardown cleans up the test database and container
func (td *TestDB) Teardown(t *testing.T) {
	if err := td.DB.Close(); err != nil {
		t.Errorf("Failed to close DB connection: %v", err)
	}
	if err := td.container.Terminate(context.Background()); err != nil {
		t.Fatalf("Failed to terminate container: %v", err)
	}
}
