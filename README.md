# GoFlow

GoFlow is a lightweight workflow management server with a CLI and HTTP API, built in Go. It’s deployed on Render and uses Supabase for persistent storage.

## Prerequisites

- **Go**: Version 1.24 or higher.
  - Install: [go.dev/dl/](https://go.dev/dl/)
    ```bash
    # Linux/Mac example
    tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    source ~/.bashrc
    go version  # Verify: go1.24...


## Installation

```bash
# Clone the repository
git clone https://github.com/ignatij/goflow.git
cd goflow

# Install Go dependencies
make deps

# Build the CLI binary
make build

# Start and migrate local PostgreSQL container
make init_db

# Ensure .env file exists with:
DB_USERNAME=testuser
DB_PASSWORD=testpass
DB_HOST=localhost
DB_PORT=5433
DB_NAME=goflow_test

# Start the HTTP server with local PostgreSQL
make start

# Start the local binary as HTTP server with local PostgreSQL
make start_binary

# Start the HTTP server with like Docker container
make build_docker
make start_docker


# List all workflows (Local environment)
./goflow list --db "postgres://testuser:testpass@localhost:5433/goflow_test?sslmode=disable"

# Run without building (Local environment)
go run cmd/goflow/main.go list --db "$DATABASE_URL"