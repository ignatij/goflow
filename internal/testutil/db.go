// internal/testutil/db.go
package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// TestDB holds the test database connection and container.
type TestDB struct {
	DB        *sqlx.DB
	Container testcontainers.Container
	ConnStr   string
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func migrationsURL() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to resolve caller path for migrations")
	}
	migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
	return "file://" + filepath.ToSlash(migrationsDir), nil
}

func startPostgresContainer(ctx context.Context, req testcontainers.ContainerRequest) (pgContainer testcontainers.Container, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("testcontainers panic: %v", r)
		}
	}()
	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

// SetupTestDB initializes a PostgreSQL container and returns a connected DB.
// In -short mode, or locally when Docker is unavailable, the test is skipped.
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping PostgreSQL integration test in -short mode")
	}

	ctx := context.Background()

	// Load .env file if present (optional; defaults are used for ephemeral testcontainers).
	_ = godotenv.Load()

	dbUsername := envOrDefault("DB_USERNAME", "testuser")
	dbPassword := envOrDefault("DB_PASSWORD", "testpass")
	dbName := envOrDefault("DB_NAME", "goflow_test")

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
	pgContainer, err := startPostgresContainer(ctx, req)
	if err != nil {
		if os.Getenv("CI") == "" {
			t.Skipf("skipping PostgreSQL integration test (Docker unavailable): %v", err)
		}
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}

	// Connection string
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUsername, dbPassword, host, port.Port(), dbName)

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
	migURL, err := migrationsURL()
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("Failed to locate migrations directory: %v", err)
	}
	m, err := migrate.New(migURL, connStr)
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
		Container: pgContainer,
		ConnStr:   connStr,
	}
}

// Teardown cleans up the test database and container
func (td *TestDB) Teardown(t *testing.T) {
	if err := td.DB.Close(); err != nil {
		t.Errorf("Failed to close DB connection: %v", err)
	}
	if err := td.Container.Terminate(context.Background()); err != nil {
		t.Fatalf("Failed to terminate container: %v", err)
	}
}
