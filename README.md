# GoFlow

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/ignatij/goflow)](https://goreportcard.com/report/github.com/ignatij/goflow)
[![Go Reference](https://pkg.go.dev/badge/github.com/ignatij/goflow.svg)](https://pkg.go.dev/github.com/ignatij/goflow)

A lightweight, high-performance workflow management library built in Go. GoFlow provides a simple yet powerful way to orchestrate complex task dependencies with support for retries, timeouts, and parallel execution. Use it as a library in your Go applications or run it as a standalone service.

## ✨ Features

- **🔄 Task Dependencies**: Define complex workflows with task dependencies using a simple API
- **⚡ Parallel Execution**: Automatic parallel execution of independent tasks
- **🛡️ Retry Mechanism**: Configurable retry policies with exponential backoff
- **⏱️ Timeout Support**: Per-task timeout configuration
- **📊 Persistence**: PostgreSQL-backed storage for workflow state and history
- **🌐 HTTP API**: RESTful API for workflow management (when used as a service)
- **🖥️ CLI Interface**: Command-line tool for workflow operations (when used as a service)
- **🐳 Docker Support**: Containerized deployment with Docker
- **🧪 Comprehensive Testing**: Extensive test coverage with mock storage

## 🚀 Quick Start

### Using GoFlow as a Library

#### Installation

Add GoFlow to your Go module:

```bash
go get github.com/ignatij/goflow@v0.2.3
```

Or add it to your `go.mod`:

```go
require github.com/ignatij/goflow v0.2.3
```

#### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/ignatij/goflow/internal/log"
    "github.com/ignatij/goflow/pkg/service"
    "github.com/ignatij/goflow/pkg/storage"
)

func main() {
    ctx := context.Background()
    logger := log.GetLogger()
    store := storage.NewMockStore()

    // Create workflow service
    wfService := service.NewWorkflowService(ctx, store, logger)

    // Register tasks
    wfService.RegisterTask("fetch_data", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        fmt.Println("Fetching data...")
        return "raw_data", nil
    }, nil)

    wfService.RegisterTask("process_data", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        fmt.Printf("Processing data: %v\n", args[0])
        return "processed_data", nil
    }, []string{"fetch_data"})

    // Register flow
    wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        return "workflow_complete", nil
    }, []string{"process_data"})

    // Create and execute workflow
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

#### Advanced Usage with Retries and Timeouts

```go
package main

import (
    "context"
    "fmt"
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

    // Register task with retry and timeout configuration
    wfService.RegisterTask("unstable_api_call", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        // Simulate API call that might fail
        if time.Now().Unix()%3 == 0 {
            return nil, fmt.Errorf("API temporarily unavailable")
        }
        return "api_response", nil
    }, nil, 
        models.WithRetries(3),           // Retry up to 3 times
        models.WithTimeout(30*time.Second), // 30 second timeout
    )

    wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        return "workflow_complete", nil
    }, []string{"unstable_api_call"})

    workflowID, err := wfService.CreateWorkflow("retry-example")
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

#### Using PostgreSQL Storage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/ignatij/goflow/internal/log"
    "github.com/ignatij/goflow/internal/storage"
    "github.com/ignatij/goflow/pkg/service"
)

func main() {
    ctx := context.Background()
    logger := log.GetLogger()
    
    // Connect to PostgreSQL
    dbURL := "postgres://username:password@localhost:5432/goflow?sslmode=disable"
    store, err := storage.NewPostgresStore(dbURL)
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()

    wfService := service.NewWorkflowService(ctx, store, logger)

    // Your workflow logic here...
    wfService.RegisterTask("persistent_task", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        return "persistent_result", nil
    }, nil)

    wfService.RegisterFlow("main", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        return "workflow_complete", nil
    }, []string{"persistent_task"})

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

### Using GoFlow as a Service

If you prefer to run GoFlow as a standalone service, you can build and run the server:

```bash
# Clone the repository
git clone https://github.com/ignatij/goflow.git
cd goflow

# Install dependencies
make deps

# Build the binary
make build

# Start local database
make init_db

# Start the service
make start
```

## 📖 Examples

The project includes comprehensive examples demonstrating various features:

- **[Basic Workflow](examples/basic_workflow/)**: Simple task execution
- **[Dependency Management](examples/dependency_workflow/)**: Complex task dependencies
- **[Error Handling](examples/error_handling/)**: Retry mechanisms and error recovery
- **[Context Timeout](examples/context_timeout/)**: Timeout handling
- **[Custom Tasks](examples/custom_task/)**: Custom task implementations
- **[Retry Mechanisms](examples/retry_always_fail/)**: Retry policies and failure handling
- **[Timeout Retries](examples/retry_with_timeout/)**: Timeout with retry configuration

## 🏗️ Architecture

### Core Components

- **WorkflowService**: Main orchestrator for workflow execution
- **WorkerPool**: Manages parallel task execution with dependency resolution
- **TaskService**: Handles task lifecycle and status management
- **Storage Interface**: Pluggable storage backend (PostgreSQL, Mock)

### Key Concepts

- **Workflow**: A collection of tasks and their dependencies
- **Task**: Individual unit of work with optional retry/timeout configuration
- **Flow**: Entry point that defines the execution path
- **Dependencies**: Task relationships that determine execution order

## 🔧 Configuration

### Task Configuration

```go
// Configure task with retries and timeout
wfService.RegisterTask("unstable_task", taskFunc, deps,
    models.WithRetries(3),
    models.WithTimeout(30*time.Second),
)
```

### Database Configuration

When using PostgreSQL storage, create a `.env` file for database configuration:

```env
DB_USERNAME=your_username
DB_PASSWORD=your_password
DB_HOST=localhost
DB_PORT=5432
DB_NAME=goflow
```

## 🛠️ Development

### Prerequisites

- Go 1.24+
- PostgreSQL (for integration tests)
- Docker (optional)

### Setup Development Environment

```bash
# Install dependencies
make deps

# Start local database
make init_db

# Run tests
make test

# Run linter
make lint

# Format code
make fmt
```

### Version Management

GoFlow uses semantic versioning with automated releases. The versioning system automatically determines the next version based on conventional commits:

```bash
# Show current version
make version

# Show next version based on commits
make version-next

# Bump version (updates files but doesn't create tag)
make version-bump

# Release version (bump, tag, and push)
make version-release
```

**Commit Types:**
- `feat:` - New features (minor version bump)
- `fix:` - Bug fixes (patch version bump)
- `chore:` - Maintenance tasks (patch version bump)
- `BREAKING CHANGE:` - Breaking changes (major version bump)
- `docs:`, `style:`, `refactor:`, `perf:`, `test:` - No version bump

For detailed information about the versioning system, see [VERSIONING.md](docs/VERSIONING.md).

### Available Make Targets

```bash
make help  # Show all available targets
```

| Target | Description |
|--------|-------------|
| `deps` | Install Go dependencies |
| `build` | Build the binary |
| `test` | Run all tests |
| `lint` | Run linter |
| `fmt` | Format code |
| `vet` | Run go vet |
| `init_db` | Start and migrate local database |
| `start` | Start the application |
| `build_docker` | Build Docker image |
| `version` | Show current version |
| `version-next` | Show next version based on commits |
| `version-bump` | Bump version based on commits |
| `version-release` | Release version (bump, tag, push) |

## 📡 API Reference

### Library API

The main types and functions for library usage:

```go
// Core types
type WorkflowService struct { ... }
type TaskResult interface{}
type ContextTaskFunc func(ctx context.Context, args ...TaskResult) (TaskResult, error)

// Main functions
func NewWorkflowService(ctx context.Context, store storage.Store, logger Logger) *WorkflowService
func (s *WorkflowService) RegisterTask(name string, fn ContextTaskFunc, deps []string, opts ...models.TaskOption) error
func (s *WorkflowService) RegisterFlow(name string, fn ContextTaskFunc, deps []string) error
func (s *WorkflowService) CreateWorkflow(name string) (int64, error)
func (s *WorkflowService) ExecuteFlow(ctx context.Context, workflowID int64, flowName string) (TaskResult, error)
```

### HTTP Endpoints (Service Mode)

When running as a service, the following RESTful endpoints are available:

- `GET /workflows` - List all workflows
- `POST /workflows` - Create a new workflow
- `GET /workflows/{id}` - Get workflow details
- `POST /workflows/{id}/execute` - Execute a workflow flow

### CLI Commands (Service Mode)

```bash
# List workflows
./goflow list

# Create workflow
./goflow create --name "my-workflow"

# Execute workflow
./goflow execute --id 1 --flow main
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style

- Follow Go conventions and best practices
- Run `make fmt` and `make lint` before submitting
- Add tests for new features
- Update documentation as needed

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with [Go](https://golang.org/)
- Database migrations with [golang-migrate](https://github.com/golang-migrate/migrate)
- Testing with [testify](https://github.com/stretchr/testify)
- CLI framework with [Cobra](https://github.com/spf13/cobra)

## 📞 Support

- 📧 Email: [your-email@example.com]
- 🐛 Issues: [GitHub Issues](https://github.com/ignatij/goflow/issues)
- 📖 Documentation: [GitHub Wiki](https://github.com/ignatij/goflow/wiki)

---

**GoFlow** - Simple, powerful workflow orchestration in Go.# Dummy commit to test workflow
