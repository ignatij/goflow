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
	go tool golangci-lint run

.PHONY: vet
vet: ## 👨‍⚕️ Vet code
	go vet ./...

.PHONY: test
test: ## 🧪 Run all tests
	go tool godotenv -f .env go test -race -count=1 -shuffle=on -coverprofile=coverage.out ./...
	
.PHONY: build_docker
build_docker: ## 👷 Build docker image
	docker build -t goflow:latest .

# not adapted yed
.PHONY: start
start: ## 🏁 Start app
	sh scripts/start-app.sh

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

.PHONY: help
help: ## 🤔 Show help messages for make targets
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(firstword $(MAKEFILE_LIST)) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
