h: help
export ENVIRONMENT=local

.PHONY: deps
deps: ## 🍚 Install dependencies
	go mod tidy && go mod vendor

.PHONY: start_db
start_db: ## 🔩 Start local database container
	bash scripts/start-db.sh start

.PHONY: init_db
init_db: ## 🔩 Start and migrate local database
	bash scripts/init-local-db.sh start

.PHONY: migrate_db
migrate_db: ## 🔩 Migrate local database
	bash scripts/init-local-db.sh migrate

.PHONY: stop_db
stop_db: ## 🔩 Stop local database
	bash scripts/init-local-db.sh stop

.PHONY: cleanup_db
cleanup_db: ## 🔩 Cleanup local database
	bash scripts/init-local-db.sh cleanup    

.PHONY: restart_db
restart_db: ## 🔩 Restart local database
	bash scripts/init-local-db.sh restart    

.PHONY: fmt
fmt: ## 🎨 Format code
	go fmt ./...

.PHONY: lint
lint: ## 🧹 Lint code
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run; \
	fi

.PHONY: vet
vet: ## 👨‍⚕️ Vet code
	go vet ./...

.PHONY: test
test: ## 🧪 Run all tests
	go tool godotenv -f .env go test -race -count=1 -shuffle=on -coverprofile=coverage.out ./...

.PHONY: test-repeat
test-repeat: ## 🔄 Run tests 10 times to check for flaky tests
	./scripts/test-repeat.sh

.PHONY: build
build: ## 👷 Build
	cd cmd/goflow && go build -o goflow main.go && mv goflow ../../

.PHONY: build_docker
build_docker: ## 👷 Build docker image
	docker build -t goflow:latest .

.PHONY: start_docker
start_docker: ## 🏁 Start docker app
	cd ci && docker-compose up goflow

.PHONY: start
start: ## 🏁 Start app
	go tool godotenv -f .env go run cmd/goflow/main.go

.PHONY: start_binary
start_binary: ## 🏁 Start app
	go tool godotenv -f .env ./goflow

# not adapted yed
.PHONY: openapi_http
openapi_http: ## 🎨 Openapi
	@./scripts/openapi-http.sh internal/http http

.PHONY: integration_tests
integration_tests: ## 🧪 Run integration tests
	sh scripts/integration-tests.sh

.PHONY: unit_tests
unit_tests: ## 🧪 Run unit tests
	sh scripts/unit-tests.sh

.PHONY: e2e_tests
e2e_tests: ## 🧪 Run e2e tests
	sh scripts/e2e-tests.sh

.PHONY: version
version: ## 📋 Show current version
	@./scripts/version.sh version

.PHONY: version-next
version-next: ## 🔮 Show next version based on commits
	@./scripts/version.sh next

.PHONY: version-bump
version-bump: ## ⬆️ Bump version based on commits
	@./scripts/version.sh bump

.PHONY: version-release
version-release: ## 🚀 Release version (bump, tag, push)
	@./scripts/version.sh release

.PHONY: help
help: ## 🤔 Show help messages for make targets
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(firstword $(MAKEFILE_LIST)) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
