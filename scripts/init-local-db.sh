#!/bin/bash

# db.sh - Manage PostgreSQL container for GoFlow

set -e # Exit on error

# Default action if none provided
ACTION=${1:-"help"}

# Load .env file from project root
if [ -f .env ]; then
    export $(cat .env | sed 's/#.*//g' | xargs)
else
    echo "Error: .env file not found in project root. Please create one with DB_USERNAME, DB_PASSWORD, DB_NAME, DB_HOST, DB_PORT."
    exit 1
fi

# Validate required environment variables
for var in DB_USERNAME DB_PASSWORD DB_NAME DB_HOST DB_PORT; do
    if [ -z "${!var}" ]; then
        echo "Error: $var is not set in .env"
        exit 1
    fi
done

# Connection string for migrations
CONN_STR="postgres://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

# Path to docker-compose.yml
COMPOSE_FILE="ci/docker-compose.yml"

# Functions
start_db() {
    echo "Starting PostgreSQL container..."
    docker-compose -f "$COMPOSE_FILE" up -d
    echo "Waiting for PostgreSQL to be ready..."
    until docker-compose -f "$COMPOSE_FILE" exec postgres pg_isready -U "${DB_USERNAME}" -d "${DB_NAME}" > /dev/null 2>&1; do
        sleep 1
    done
    echo "PostgreSQL is up."
}

migrate_db() {
    echo "Running migrations..."
    go run cmd/goflow-migrate/main.go migrate
}

stop_db() {
    echo "Stopping PostgreSQL container..."
    docker-compose -f "$COMPOSE_FILE" down
}

cleanup_db() {
    echo "Cleaning up PostgreSQL container and volume..."
    docker-compose -f "$COMPOSE_FILE" down -v --remove-orphans
}

# Handle actions
case "$ACTION" in
    start)
        start_db
        migrate_db
        echo "Database is running and migrated. Connect with: $CONN_STR"
        ;;
    stop)
        stop_db
        echo "Database stopped."
        ;;
    cleanup)
        cleanup_db
        echo "Database state cleaned up."
        ;;
    restart)
        stop_db
        start_db
        migrate_db
        echo "Database restarted and migrated."
        ;;
    help)
        echo "Usage: $0 {start|stop|cleanup|restart}"
        echo "  start   - Start the PostgreSQL container and run migrations"
        echo "  stop    - Stop the PostgreSQL container"
        echo "  cleanup - Stop and remove the container and volume"
        echo "  restart - Stop, start, and migrate the database"
        exit 0
        ;;
    *)
        echo "Unknown action: $ACTION"
        echo "Usage: $0 {start|stop|cleanup|restart}"
        exit 1
        ;;
esac