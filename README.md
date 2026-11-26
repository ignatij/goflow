# GoFlow

[![CI](https://github.com/ignatij/goflow/actions/workflows/ci.yml/badge.svg)](https://github.com/ignatij/goflow/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26+-blue.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/ignatij/goflow)](https://goreportcard.com/report/github.com/ignatij/goflow)
[![Go Reference](https://pkg.go.dev/badge/github.com/ignatij/goflow.svg)](https://pkg.go.dev/github.com/ignatij/goflow)

GoFlow is a Go workflow execution library for orchestrating in-process task DAGs with dependency resolution, bounded parallel execution, per-attempt context timeouts, jittered exponential-backoff retries, and PostgreSQL-backed task state persistence.

## Design & Guarantees

GoFlow is intentionally scoped as an embedded Go workflow library rather than a distributed orchestrator:

- **Static, code-registered DAGs:** Tasks and flows are ordinary Go functions registered on a `WorkflowService`. Before execution starts, `ExecuteFlow` runs Kahn's topological sort to reject dependency cycles and missing tasks up front, then prunes the graph to nodes reachable from the requested flow.
- **Bounded concurrency:** Ready tasks execute across a fixed worker pool (`runtime.NumCPU()` goroutines by default). Independent branches run in parallel; dependent tasks wait until all upstream dependencies finish and receive upstream outputs in dependency order.
- **Per-attempt timeouts & cancellation:** Each attempt runs with `context.WithTimeout` (default `60s`, configurable via `models.WithTimeout`) derived from the caller's context and the service lifecycle context. Context cancellation aborts in-flight backoff sleeps promptly and marks pending tasks failed.
- **Exponential backoff with equal jitter:** Failed or timed-out attempts retry after a delay in `[d/2, d]` where `d = min(base * 2^attempt, max)` (defaults: `100ms` base, `30s` cap; configurable via `models.WithRetryBackoff`). Retries are **at-least-once**, so task functions must be idempotent.
- **Persistence boundary & non-goals:** Storage sits behind `pkg/storage.Store`, with an in-memory mock (`pkg/storage.NewMockStore`) and a PostgreSQL implementation (`pkg/storage/postgres.NewStore`) that records workflow status, task statuses, dependency edges, attempt counts, and error messages. Task return values (`TaskResult`) stay in memory during a run, and GoFlow does **not** replay or resume workflows across process restarts.

## Quick Start

### Installation

```bash
go get github.com/ignatij/goflow@v0.2.7
```

### Basic DAG Execution

`WorkflowService` accepts any logger implementing `service.Logger` (`Infof` and `Errorf`) and any `storage.Store`:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage"
)

type stdLogger struct{}

func (stdLogger) Infof(format string, args ...interface{})  { log.Printf("[INFO] "+format, args...) }
func (stdLogger) Errorf(format string, args ...interface{}) { log.Printf("[ERROR] "+format, args...) }

func main() {
	ctx := context.Background()
	store := storage.NewMockStore()

	wfService := service.NewWorkflowService(ctx, store, stdLogger{})

	if err := wfService.RegisterTask("fetch_data", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		return "raw_data", nil
	}, nil); err != nil {
		log.Fatal(err)
	}

	if err := wfService.RegisterTask("process_data", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		return fmt.Sprintf("processed(%v)", args[0]), nil
	}, []string{"fetch_data"}); err != nil {
		log.Fatal(err)
	}

	if err := wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		return args[0], nil
	}, []string{"process_data"}); err != nil {
		log.Fatal(err)
	}

	workflowID, err := wfService.CreateWorkflow("example-workflow")
	if err != nil {
		log.Fatal(err)
	}

	result, err := wfService.ExecuteFlow(ctx, workflowID, "main")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Result: %v\n", result)
}
```

### Retries, Backoff, and Timeouts

```go
err := wfService.RegisterTask(
	"unstable_api_call",
	func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		// Respect ctx.Done() inside long-running I/O calls.
		return "api_response", nil
	},
	nil,
	models.WithRetries(3),
	models.WithRetryBackoff(100*time.Millisecond, 5*time.Second),
	models.WithTimeout(30*time.Second),
)
```

### PostgreSQL State Persistence

Apply the SQL migrations in [`migrations/`](migrations/) (e.g., via `cmd/goflow-migrate`) and pass `postgres.NewStore` to `WorkflowService`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ignatij/goflow/pkg/service"
	"github.com/ignatij/goflow/pkg/storage/postgres"
)

type stdLogger struct{}

func (stdLogger) Infof(format string, args ...interface{})  { log.Printf("[INFO] "+format, args...) }
func (stdLogger) Errorf(format string, args ...interface{}) { log.Printf("[ERROR] "+format, args...) }

func main() {
	ctx := context.Background()

	store, err := postgres.NewStore(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	wfService := service.NewWorkflowService(ctx, store, stdLogger{})

	if err := wfService.RegisterTask("persistent_task", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		return "persistent_result", nil
	}, nil); err != nil {
		log.Fatal(err)
	}

	if err := wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
		return args[0], nil
	}, []string{"persistent_task"}); err != nil {
		log.Fatal(err)
	}

	workflowID, err := wfService.CreateWorkflow("persistent-workflow")
	if err != nil {
		log.Fatal(err)
	}

	result, err := wfService.ExecuteFlow(ctx, workflowID, "main")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Result: %v\n", result)
}
```

## Architecture

```text
pkg/
├── models/             # Workflow, Task, Dependency, TaskOption (WithRetries, WithTimeout, WithRetryBackoff)
├── service/
│   ├── service.go      # WorkflowService: registration, Kahn topological sort, flow execution lifecycle
│   ├── worker_pool.go  # WorkerPool: bounded workers, per-attempt timeout, exponential backoff retries
│   └── task_service.go # Transactional task persistence and dependency-readiness checks
└── storage/
    ├── storage.go      # Store interface & ErrNotFound
    ├── mock_storage.go # In-memory Store for unit tests
    └── postgres/       # PostgreSQL Store implementation (sqlx + lib/pq)
```

## Examples

Runnable examples in [`examples/`](examples/):

- [`basic_workflow`](examples/basic_workflow/) — minimal task + flow execution
- [`dependency_workflow`](examples/dependency_workflow/) — parallel branches feeding downstream tasks
- [`custom_task`](examples/custom_task/) — custom task signatures and results
- [`error_handling`](examples/error_handling/) — error propagation and retries
- [`retry_always_fail`](examples/retry_always_fail/) — retry exhaustion handling
- [`retry_with_timeout`](examples/retry_with_timeout/) — combining retries with per-attempt timeouts
- [`context_timeout`](examples/context_timeout/) — per-task timeout enforcement
- [`main_context_timeout`](examples/main_context_timeout/) — cancelling an in-flight workflow via caller context

For a larger end-to-end sample (order validation, inventory checks with retries, payment timeouts, shipping, notifications), see [ignatij/goflow-example](https://github.com/ignatij/goflow-example/).

## Optional HTTP & CLI Shell

`cmd/goflow` includes a minimal HTTP server and CLI for inspecting and mutating persisted workflow records against PostgreSQL. Because tasks and flows are registered in Go code, `cmd/goflow` does not pre-register executable flows—embed `service.WorkflowService` in your own binary to expose domain flows.

**HTTP routes (`internal/http`):**

- `GET /health` — liveness check
- `GET /workflows` — list workflows
- `POST /workflows` — create a workflow (`{"name": "..."}`)
- `PUT /workflows` — update workflow status (`{"id": 1, "status": "PENDING|RUNNING|COMPLETED|FAILED"}`)
- `GET /workflows/{id}` — fetch workflow with persisted tasks
- `POST /workflows/{id}/flows/{flowName}` — execute a registered flow

**CLI subcommands (`./goflow --db <dsn>`):**

```bash
./goflow list
./goflow create name=my-workflow
./goflow update id=1 status=COMPLETED
```

## Development & Testing

**Prerequisites:** Go 1.26+, Docker (only for PostgreSQL integration tests via `testcontainers-go`), and optionally [golangci-lint](https://golangci-lint.run/docs/welcome/install/).

```bash
# Fast unit tests (no Docker required)
go test -short -race ./...

# Full suite including PostgreSQL integration tests (skips DB tests locally if Docker is unavailable)
go test -race -count=1 -shuffle=on ./...

# Static checks
go vet ./...
make lint
```

### Database Migrations

```bash
go run ./cmd/goflow-migrate migrate --db "postgres://user:pass@localhost:5432/goflow?sslmode=disable"
```

## API Summary

```go
type Logger interface {
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type TaskResult interface{}
type ContextTaskFunc func(ctx context.Context, args ...TaskResult) (TaskResult, error)

func NewWorkflowService(ctx context.Context, store storage.Store, logger Logger) *WorkflowService
func (s *WorkflowService) RegisterTask(name string, fn ContextTaskFunc, deps []string, opts ...models.TaskOption) error
func (s *WorkflowService) RegisterFlow(name string, fn ContextTaskFunc, deps []string) error
func (s *WorkflowService) CreateWorkflow(name string) (int64, error)
func (s *WorkflowService) ExecuteFlow(ctx context.Context, workflowID int64, flowName string) (TaskResult, error)
func (s *WorkflowService) GetWorkflow(workflowID int64) (models.Workflow, error)
func (s *WorkflowService) ListWorkflows() ([]models.Workflow, error)
```

For version management details, see [docs/VERSIONING.md](docs/VERSIONING.md).

## License

MIT — see [LICENSE](LICENSE).
