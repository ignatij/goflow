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

start_db() {
    echo "Starting PostgreSQL container..."
    docker-compose -f "$COMPOSE_FILE" up -d postgres
    echo "Waiting for PostgreSQL to be ready..."
    until docker-compose -f "$COMPOSE_FILE" exec postgres pg_isready -U "${DB_USERNAME}" -d "${DB_NAME}" > /dev/null 2>&1; do
        sleep 1
    done
    echo "PostgreSQL is up."
}

# Handle actions
case "$ACTION" in
    start)
        start_db
        echo "Database container is up and running"
        ;;
    help)
        echo "Usage: $0 {start}"
        echo "  start   - Start the PostgreSQL container"
        exit 0
        ;;
    *)
        echo "Unknown action: $ACTION"
        echo "Usage: $0 {start|stop|cleanup|restart}"
        exit 1
        ;;
esac